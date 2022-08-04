package fit

// https://developer.garmin.com/fit/protocol/
import (
	"github.com/wader/fq/format"
	"github.com/wader/fq/pkg/decode"
	"github.com/wader/fq/pkg/interp"
	"github.com/wader/fq/pkg/scalar"
)

const (
	architectureTypeLittleEndian = 0
	architectureTypeBigEndian    = 1
)

var architectureTypeMap = scalar.UToSymStr{
	architectureTypeLittleEndian: "little_endian",
	architectureTypeBigEndian:    "big_endian",
}

const (
	messageTypeData       = 0
	messageTypeDefinition = 1
)

var messageTypeMap = scalar.UToSymStr{
	messageTypeData:       "data",
	messageTypeDefinition: "definition",
}

const (
	baseTypeEnum    = 0
	baseTypeSint8   = 1
	baseTypeUint8   = 2
	baseTypeSint16  = 3
	baseTypeUint16  = 4
	baseTypeSint32  = 5
	baseTypeUint32  = 6
	baseTypeString  = 7
	baseTypeFloat32 = 8
	baseTypeFloat64 = 9
	baseTypeUint8z  = 10
	baseTypeUint16z = 11
	baseTypeUint32z = 12
	baseTypeByte    = 13
	baseTypeSint64  = 14
	baseTypeUint64  = 15
	baseTypeUint64z = 16
)

var baseTypeMap = scalar.UToSymStr{
	baseTypeEnum:    "enum",
	baseTypeSint8:   "sint8",
	baseTypeUint8:   "uint8",
	baseTypeSint16:  "sint16",
	baseTypeUint16:  "uint16",
	baseTypeSint32:  "sint32",
	baseTypeUint32:  "uint32",
	baseTypeString:  "string",
	baseTypeFloat32: "float32",
	baseTypeFloat64: "float64",
	baseTypeUint8z:  "uint8z",
	baseTypeUint16z: "uint16z",
	baseTypeUint32z: "uint32z",
	baseTypeByte:    "byte",
	baseTypeSint64:  "sint64",
	baseTypeUint64:  "uint64",
	baseTypeUint64z: "uint64z",
}

type baseType struct {
}

// Base Type #	Endian Ability	Base Type Field	Type Name	Invalid Value	Size (Bytes)	Comment
// 0	0	0x00	enum	0xFF	1
// 1	0	0x01	sint8	0x7F	1	2’s complement format
// 2	0	0x02	uint8	0xFF	1
// 3	1	0x83	sint16	0x7FFF	2	2’s complement format
// 4	1	0x84	uint16	0xFFFF	2
// 5	1	0x85	sint32	0x7FFFFFFF	4	2’s complement format
// 6	1	0x86	uint32	0xFFFFFFFF	4
// 7	0	0x07	string	0x00	1	Null terminated string encoded in UTF-8 format
// 8	1	0x88	float32	0xFFFFFFFF	4
// 9	1	0x89	float64	0xFFFFFFFFFFFFFFFF	8
// 10	0	0x0A	uint8z	0x00	1
// 11	1	0x8B	uint16z	0x0000	2
// 12	1	0x8C	uint32z	0x00000000	4
// 13	0	0x0D	byte	0xFF	1	Array of bytes. Field is invalid if all bytes are invalid.
// 14	1	0x8E	sint64	0x7FFFFFFFFFFFFFFF	8	2’s complement format
// 15	1	0x8F	uint64	0xFFFFFFFFFFFFFFFF	8
// 16	1	0x90	uint64z	0x0000000000000000	8

type field struct {
	number        uint8
	size          uint8
	endianAbility uint8
	baseType      uint8
}

type definition struct {
	architecture        int
	globalMessageNumber int
	fields              []field
}

func init() {
	interp.RegisterFormat(decode.Format{
		Name:        format.FIT,
		Description: "Flexible and Interoperable Data Transfer (FIT) Protocol",
		DecodeFn:    decodeFIT,
	})
}

func decodeDataMessage(d *decode.D, de definition) {
	d.FieldArray("fields", func(d *decode.D) {
		for _, e := range de.fields {
			d.FieldArray("values", func(d *decode.D) {
				switch e.baseType {
				case baseTypeEnum:
					d.FieldU8("value")
				case baseTypeSint8:
					d.FieldS8("value")
				case baseTypeUint8:
					d.FieldU8("value")
				case baseTypeSint16:
					d.FieldS16("value")
				case baseTypeUint16:
					d.FieldU16("value")
				case baseTypeSint32:
					d.FieldU32("value")
				case baseTypeUint32:
					d.FieldU32("value")
				case baseTypeString:
					d.FieldU8("value") // TODO:
				case baseTypeFloat32:
					d.FieldF32("value")
				case baseTypeFloat64:
					d.FieldF64("value")
				case baseTypeUint8z:
					d.FieldU8("value")
				case baseTypeUint16z:
					d.FieldU8("value")
				case baseTypeUint32z:
					d.FieldU8("value")
				case baseTypeByte:
					d.FieldU8("value")
				case baseTypeSint64:
					d.FieldU8("value")
				case baseTypeUint64:
					d.FieldU8("value")
				case baseTypeUint64z:
					d.FieldU8("value")
				default:
					panic("unreachable")
				}
			})
		}
	})
}

func decodeFieldDefinition(d *decode.D) field {
	var field field
	field.number = uint8(d.FieldU8("field_definition_number"))
	field.size = uint8(d.FieldU8("size"))
	field.endianAbility = uint8(d.FieldU1("endian_ability"))
	d.RawLen(2)
	field.baseType = uint8(d.FieldU5("base_type_number", baseTypeMap))

	return field
}

func decodeDefinitionMessage(d *decode.D) definition {
	var de definition
	d.FieldU8("reserved")
	de.architecture = int(d.FieldU8("architecture"))
	de.globalMessageNumber = int(d.FieldU16("global_message_number"))
	numFields := d.FieldU8("fields")
	d.FieldArray("field_definitions", func(d *decode.D) {
		for i := uint64(0); i < numFields; i++ {
			d.FieldStruct("field_definition", func(d *decode.D) {
				de.fields = append(de.fields, decodeFieldDefinition(d))
			})
		}
	})

	return de
}

func decodeFIT(d *decode.D, in interface{}) interface{} {
	d.Endian = decode.LittleEndian

	// TODO: chained files

	definitions := map[int]definition{}

	size := d.FieldU8("header_size")
	if size < 12 {
		d.Fatalf("Header size too small %d < 12", size)
	}
	d.FieldU8("protocol_version")
	d.FieldU16("profile_version")
	dataSize := d.FieldU32("data_size")
	d.FieldUTF8("data_type", 4, d.AssertStr(".FIT"))
	d.FieldU16("crc")

	d.FramedFn(int64(dataSize)*8, func(d *decode.D) {
		d.FieldArray("records", func(d *decode.D) {
			for !d.End() {
				d.FieldStruct("record_header", func(d *decode.D) {
					d.FieldU1("normal_header")
					messageType := d.FieldU1("message_type", messageTypeMap)
					d.FieldU1("message_type_specific")
					d.FieldU1("reserved")
					localMessageType := int(d.FieldU4("local_message_type"))
					d.FieldStruct("message", func(d *decode.D) {
						switch messageType {
						case messageTypeData:
							if de, ok := definitions[localMessageType]; ok {
								decodeDataMessage(d, de)
							} else {
								d.Fatalf("unkown local message type %d", localMessageType)
							}
						case messageTypeDefinition:
							definitions[localMessageType] = decodeDefinitionMessage(d)
						default:
							panic("unreachable")
						}
					})
				})
			}
		})
	})

	return nil
}
