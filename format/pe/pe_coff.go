package pe

// https://osandamalith.com/2020/07/19/exploring-the-ms-dos-stub/

import (
	"github.com/wader/fq/format"
	"github.com/wader/fq/pkg/decode"
	"github.com/wader/fq/pkg/interp"
	"github.com/wader/fq/pkg/scalar"
)

// TODO: probe?

func init() {
	interp.RegisterFormat(decode.Format{
		Name:        format.PE_COFF, // TODO: not PE_ prefix?
		Description: "Common Object File Format",
		DecodeFn:    peCoffStubDecode,
	})
}

func peCoffStubDecode(d *decode.D, _ any) any {

	d.FieldU32("signature", scalar.UintHex, d.UintAssert(0x50450000))
	d.FieldU16("machine")
	d.FieldU16("number_of_sections")
	d.FieldU32("time_date_stamp")
	d.FieldU32("pointer_to_symbol_table")
	d.FieldU32("number_of_symbol_table")
	d.FieldU16("size_of_optional_header")
	d.FieldU16("characteristics")

	return nil
}
