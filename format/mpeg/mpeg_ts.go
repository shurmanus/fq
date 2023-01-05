package mpeg

// TODO: dump bug: array with only sub buffers show wrong summary bytes
// TODO: dump idea: array with only value scalars, collapse?
// TODO: split into generic table decoder or helper function? and use table_id to select? pass on args and return out value? switch on return type?
// TODO: probe, count?
// TODO: check crc
// TODO: mpeg_pes, share code?
// TODO: mpeg_pes_packet, length 0 for video?

// ffmpeg $(for i in $(seq 0 50); do echo "-f lavfi -i sine"; done) -t 100ms $(for i in $(seq 0 50); do echo "-map $i:0"; done) test2.ts

// ISO/IEC 13818-1 - Generic coding of moving pictures and associated audio information: Systems
// https://tsduck.io/download/docs/mpegts-introduction.pdf

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/wader/fq/format"
	"github.com/wader/fq/pkg/bitio"
	"github.com/wader/fq/pkg/decode"
	"github.com/wader/fq/pkg/interp"
	"github.com/wader/fq/pkg/scalar"
)

var mpegTsFormatMpegTsPacket decode.Group
var mpegTsFormatMpegTsPat decode.Group
var mpegTsFormatMpegTsPmt decode.Group
var mpegTsFormatMpegPesPacket decode.Group

func init() {
	interp.RegisterFormat(decode.Format{
		Name:        format.MPEG_TS,
		Description: "MPEG Transport Stream",
		Groups:      []string{format.PROBE},
		Dependencies: []decode.Dependency{
			{Names: []string{format.MPEG_TS_PACKET}, Group: &mpegTsFormatMpegTsPacket},
			{Names: []string{format.MPEG_TS_PAT}, Group: &mpegTsFormatMpegTsPat},
			{Names: []string{format.MPEG_TS_PMT}, Group: &mpegTsFormatMpegTsPmt},
			{Names: []string{format.MPEG_PES_PACKET}, Group: &mpegTsFormatMpegPesPacket},
		},
		DecodeFn: tsDecode,
	})
}

const (
	adaptationFieldControlPayloadOnly               = 0b01
	adaptationFieldControlAdaptationFieldOnly       = 0b10
	adaptationFieldControlAdaptationFieldAndPayload = 0b11
)

type tsBuffer struct {
	length        int
	buf           bytes.Buffer
	packetIndexes []int
}

func (tb *tsBuffer) Reset() {
	tb.length = -1
	// new bytes buffer to not share byte slice
	tb.buf = bytes.Buffer{}
	tb.packetIndexes = nil

}

type tsContinuityMap map[int]int

func (tcm tsContinuityMap) Update(pid int, n int) bool {
	current, currentOk := tcm[pid]
	tcm[pid] = n
	if currentOk {
		return (current+1)&0xf == n
	}
	return n == 0
}

func tsPesDecode(d *decode.D, pid int, programPid int, streamType int, pesBuf *tsBuffer) {
	d.FieldValueUint("pid", uint64(pid), tsPidMap, scalar.UintHex)                       // TODO: more things? or less?
	d.FieldValueUint("program", uint64(programPid), scalar.UintHex)                      // TODO: more things? or less?
	d.FieldValueUint("stream_type", uint64(streamType), tsStreamTypeMap, scalar.UintHex) // TODO: more things? or less?
	d.FieldArray("indexes", func(d *decode.D) {
		for _, i := range pesBuf.packetIndexes {
			d.FieldValueUint("index", uint64(i))
		}
	})
	d.FieldRawLen("payload", d.BitsLeft())
	// d.TryFieldFormatBitBuf("payload", bitio.NewBitReader(b.Bytes(), -1), mpegTsFormatMpegPesPacket, nil)
}

func tsDecode(d *decode.D, _ any) any {
	var tableReassemble = map[int]*tsBuffer{}
	var pesReassemble = map[int]*tsBuffer{}
	pidProgramMap := map[int]format.MpegTsProgram{}
	pidStreamMap := map[int]format.MpegTsStream{}
	continuityMap := tsContinuityMap(map[int]int{})
	packetIndex := 0

	tablesD := d.FieldArrayValue("tables")
	pesD := d.FieldArrayValue("pes")

	d.FieldArray("packets", func(d *decode.D) {
		for !d.End() {
			_, v, err := d.TryFieldFormat(
				"packet",
				mpegTsFormatMpegTsPacket,
				format.MpegTsPacketIn{
					ProgramMap: pidProgramMap,
					StreamMap:  pidStreamMap,
				},
			)
			if err != nil {
				// TODO: malformted packet, how?
				d.FieldRawLen("packet", tsPacketLength)
				continue
			}
			mtpo, mtpoOk := v.(format.MpegTsPacketOut)
			if !mtpoOk {
				panic("packet is not a MpegTsPacketOut")
			}

			isContinous := continuityMap.Update(mtpo.Pid, mtpo.ContinuityCounter)
			isTable := tsPidIsTable(mtpo.Pid, pidProgramMap)
			stream, isStream := pidStreamMap[mtpo.Pid]

			// log.Printf("mtpo.Pid: %x isContinous=%t isTable=%t isStream=%t", mtpo.Pid, isContinous, isTable, isStream)

			switch {
			case isTable:
				if !isContinous {
					// TODO: reset
				}

				// TODO: version, current section etc
				tableBuf, tableBufOk := tableReassemble[mtpo.Pid]
				if !tableBufOk {
					tableBuf = &tsBuffer{length: -1}
					tableReassemble[mtpo.Pid] = tableBuf
				}
				tableBuf.packetIndexes = append(tableBuf.packetIndexes, packetIndex)
				b := &tableBuf.buf
				b.Write(mtpo.Payload)

				// log.Printf("  b.Len() 1: %p %#+v %d\n", &b, b.Len(), tableBuf.length)

				const sectionHeaderLength = 3
				if tableBuf.length == -1 && b.Len() >= sectionHeaderLength {
					// length is BE 10 bits of byte 1 and 2 and add header length to know expected full length
					tableBuf.length = int(binary.BigEndian.Uint16(b.Bytes()[1:])&0b0000_0011_1111_1111) + sectionHeaderLength
				}

				// log.Printf("  b.Len() 2: %#+v %d\n", b.Len(), tableBuf.length)

				if b.Len() >= tableBuf.length {
					program, isPMT := pidProgramMap[mtpo.Pid]

					tablesD.FieldStructRootBitBufFn("table", bitio.NewBitReader(b.Bytes(), -1), func(d *decode.D) {
						d.FieldValueUint("pid", uint64(mtpo.Pid), tsPidMap, scalar.UintHex) // TODO: more things? or less?
						d.FieldArray("indexes", func(d *decode.D) {
							for _, i := range tableBuf.packetIndexes {
								d.FieldValueUint("index", uint64(i))
							}
						})

						switch {
						case mtpo.Pid == pidPAT:
							_, v, err := d.TryFieldFormat("payload", mpegTsFormatMpegTsPat, nil)
							if err != nil {
								// TODO: malformted table, how?
								d.FieldRawLen("payload", tsPacketLength)
							} else {
								mtpo, mtpoOk := v.(format.MpegTsPatOut)
								if !mtpoOk {
									panic(fmt.Sprintf("expected MpegTsPatOut got %#+v", v))
								}

								// TODO: correct? remove streams for program?
								for mapPid, mapNum := range mtpo.PidMap {
									if prevProgram, ok := pidProgramMap[mapPid]; ok {
										for _, streamPid := range prevProgram.StreamPids {
											delete(pidStreamMap, streamPid)
										}
									}
									pidProgramMap[mapPid] = format.MpegTsProgram{
										Number: mapNum,
										Pid:    mapPid,
									}
								}
							}

						case isPMT:
							_, v, err := d.TryFieldFormat("payload", mpegTsFormatMpegTsPmt, nil)
							if err != nil {
								// TODO: malformted table, how?
								d.FieldRawLen("packet", tsPacketLength)
							} else {

								mtpo, mtpoOk := v.(format.MpegTsPmtOut)
								if !mtpoOk {
									panic(fmt.Sprintf("expected MpegTsPmtOut got %#+v", v))
								}

								// TODO: correct? replace streams?
								for _, streamPid := range program.StreamPids {
									delete(pidStreamMap, streamPid)
								}
								for streamPid, stream := range mtpo.Streams {
									program.StreamPids = append(program.StreamPids, streamPid)
									pidStreamMap[streamPid] = format.MpegTsStream{
										ProgramPid: program.Pid,
										Type:       stream.Type,
									}
								}
							}

						default:
							// TODO: raw table decoder?
							d.FieldRawLen("payload", d.BitsLeft())
						}

						tableBuf.Reset()
					})
				}

			case isStream:
				if !isContinous {
					// TODO: reset
				}

				pesBuf, pesBufOk := pesReassemble[mtpo.Pid]
				if !pesBufOk {
					pesBuf = &tsBuffer{length: -1}
					pesReassemble[mtpo.Pid] = pesBuf
				}
				b := &pesBuf.buf

				pesFn := func(d *decode.D) {
					tsPesDecode(d, mtpo.Pid, stream.ProgramPid, stream.Type, pesBuf)
					pesBuf.Reset()
				}

				// TODO: when to reset if wrong?
				if mtpo.PayloadUnitStart && pesBuf.length == 0 && b.Len() > 0 {
					// TODO: only video?

					pesD.FieldStructRootBitBufFn("pes", bitio.NewBitReader(b.Bytes(), -1), pesFn)
				}

				pesBuf.packetIndexes = append(pesBuf.packetIndexes, packetIndex)
				b.Write(mtpo.Payload)

				const pesHeaderLength = 6 // 3 sync, 1 stream id, 2 length
				if pesBuf.length == -1 && b.Len() >= pesHeaderLength {
					// length is BE bytes 4 and 5 and add ad header length to know expected full length
					length := int(binary.BigEndian.Uint16(b.Bytes()[4:]))
					if length == 0 {
						pesBuf.length = 0
					} else {
						pesBuf.length = length + pesHeaderLength
					}
				}

				// log.Printf("  b.Len() 1: %p %#+v %d\n", &b, b.Len(), pesBuf.length)

				// TODO: zero length, start flag?
				if pesBuf.length > 0 && b.Len() >= pesBuf.length {
					pesD.FieldStructRootBitBufFn("pes", bitio.NewBitReader(b.Bytes(), -1), pesFn)
				}

			default:
				// unknown ts packet payload
			}

			packetIndex++
		}
	})

	// TODO:
	// add possible partial pes
	for pid, pesBuf := range pesReassemble {
		if pesBuf.buf.Len() == 0 {
			continue
		}

		// TODO: can we assume there is a stream?
		stream, isStream := pidStreamMap[pid]
		if !isStream {
			continue
		}

		pesD.FieldStructRootBitBufFn("pes", bitio.NewBitReader(pesBuf.buf.Bytes(), -1), func(d *decode.D) {
			tsPesDecode(d, pid, stream.ProgramPid, stream.Type, pesBuf)
		})

	}

	return nil
}
