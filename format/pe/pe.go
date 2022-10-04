package pe

// https://osandamalith.com/2020/07/19/exploring-the-ms-dos-stub/

import (
	"github.com/wader/fq/format"
	"github.com/wader/fq/pkg/decode"
	"github.com/wader/fq/pkg/interp"
)

// TODO: probe?

var peMSDosStubFormat decode.Group
var peCOFFFormat decode.Group

func init() {
	interp.RegisterFormat(decode.Format{
		Name:        format.PE, // TODO: not PE_ prefix?
		Description: "Portable Executable",
		Groups:      []string{format.PROBE},
		Dependencies: []decode.Dependency{
			{Names: []string{format.PE_MSDOS_STUB}, Group: &peMSDosStubFormat},
			{Names: []string{format.PE_COFF}, Group: &peCOFFFormat},
		},
		DecodeFn: peDecode,
	})
}

func peDecode(d *decode.D, _ any) any {

	d.FieldFormat("ms_dos_stub", peMSDosStubFormat, nil)
	d.FieldFormat("coff", peCOFFFormat, nil)

	return nil
}
