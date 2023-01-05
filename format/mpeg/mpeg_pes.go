package mpeg

// TODO: probeable?

// http://dvdnav.mplayerhq.hu/dvdinfo/mpeghdrs.html

import (
	"log"

	"github.com/wader/fq/format"
	"github.com/wader/fq/pkg/bitio"
	"github.com/wader/fq/pkg/decode"
	"github.com/wader/fq/pkg/interp"
)

var pesPacketFormat decode.Group
var spuFormat decode.Group

func init() {
	interp.RegisterFormat(decode.Format{
		Name:        format.MPEG_PES,
		Description: "MPEG Packetized elementary stream",
		DecodeFn:    pesDecode,
		// RootArray:   true,
		RootName: "packets",
		Dependencies: []decode.Dependency{
			{Names: []string{format.MPEG_PES_PACKET}, Group: &pesPacketFormat},
			{Names: []string{format.MPEG_SPU}, Group: &spuFormat},
		},
	})
}

type subStream struct {
	b []byte
	l int
}

func pesDecode(d *decode.D, _ any) any {
	substreams := map[int]*subStream{}

	prefix := d.PeekBits(24)
	if prefix != 0b0000_0000_0000_0000_0000_0001 {
		d.Errorf("no pes prefix found")
	}

	i := 0

	spuD := d.FieldArrayValue("spus")

	d.FieldArray("packets", func(d *decode.D) {
		for d.NotEnd() {
			dv, v, err := d.TryFieldFormat("packet", pesPacketFormat, nil)
			if dv == nil || err != nil {
				log.Printf("err: %#+v\n", err)
				break
			}

			switch dvv := v.(type) {
			case subStreamPacket:
				s, ok := substreams[dvv.number]
				if !ok {
					s = &subStream{}
					substreams[dvv.number] = s
				}
				s.b = append(s.b, dvv.buf...)

				if s.l == 0 && len(s.b) >= 2 {
					s.l = int(s.b[0])<<8 | int(s.b[1])
					// TODO: zero l?
				}

				// TODO: is this how spu end is signalled?
				if s.l == len(s.b) {
					spuD.FieldFormatBitBuf("spu", bitio.NewBitReader(s.b, -1), spuFormat, nil)
					s.b = nil
					s.l = 0
				}
			}

			i++
		}
	})

	return nil
}
