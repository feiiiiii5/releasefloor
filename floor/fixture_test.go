package floor

import "github.com/feiiiiii5/releasefloor/internal/testformat"

type fixtureNode = testformat.Node

func fixtureELF(nodes []fixtureNode, relr bool) []byte { return testformat.ELF(nodes, relr) }
func fixtureMach(cpu, subtype, min, sdk uint32, legacy bool, commands ...[]byte) []byte {
	return testformat.Mach(cpu, subtype, min, sdk, legacy, commands...)
}
func fixtureDylib(cmd uint32, name string) []byte { return testformat.Dylib(cmd, name) }
func fixtureFat(slices ...[]byte) []byte          { return testformat.Fat(slices...) }
