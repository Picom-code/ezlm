package integrate

import (
	"debug/elf"
	"debug/macho"
	"debug/pe"
)

// BinaryArch reads a program's architecture from its header: "arm64",
// "amd64", or "" when it can't tell (a script, an unknown format). A
// universal Mac binary reports MachineArch() when it contains that slice.
func BinaryArch(path string) string {
	if f, err := pe.Open(path); err == nil {
		defer f.Close()
		switch f.Machine {
		case pe.IMAGE_FILE_MACHINE_ARM64:
			return "arm64"
		case pe.IMAGE_FILE_MACHINE_AMD64:
			return "amd64"
		}
		return ""
	}
	if f, err := macho.Open(path); err == nil {
		defer f.Close()
		return machoArch(f.Cpu)
	}
	if f, err := macho.OpenFat(path); err == nil {
		defer f.Close()
		want := MachineArch()
		for _, a := range f.Arches {
			if machoArch(a.Cpu) == want {
				return want
			}
		}
		if len(f.Arches) > 0 {
			return machoArch(f.Arches[0].Cpu)
		}
		return ""
	}
	if f, err := elf.Open(path); err == nil {
		defer f.Close()
		switch f.Machine {
		case elf.EM_AARCH64:
			return "arm64"
		case elf.EM_X86_64:
			return "amd64"
		}
	}
	return ""
}

func machoArch(c macho.Cpu) string {
	switch c {
	case macho.CpuArm64:
		return "arm64"
	case macho.CpuAmd64:
		return "amd64"
	}
	return ""
}
