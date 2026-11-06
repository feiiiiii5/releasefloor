# Object-format references

The implementation was written for this project using public object-format definitions and the Go standard library. Competing tools were researched for product positioning; their implementation was not copied.

- [Go debug/elf](https://pkg.go.dev/debug/elf) documents ELF constants, dynamic symbols and version requirements. The parser cross-checks bounded section metadata against loader-facing dynamic addresses. It preserves version-provider and symbol evidence instead of searching for version-looking text anywhere in the file.
- [GNU symbol versioning](https://sourceware.org/binutils/docs/ld/VERSION.html) describes version nodes and symbol dependencies. A recorded requirement is not a guarantee that the target has every required library or that execution will succeed.
- [Apple Mach-O loader definitions](https://github.com/apple-oss-distributions/xnu/blob/main/EXTERNAL_HEADERS/mach-o/loader.h) define deployment targets and library commands. A build SDK version is kept separate from the minimum OS. Weak, reexported, lazy and upward library commands remain visible.
- [Go archive/zip](https://pkg.go.dev/archive/zip), [archive/tar](https://pkg.go.dev/archive/tar) and [compress/gzip](https://pkg.go.dev/compress/gzip) supply archive readers. ReleaseFloor adds explicit entry, size, framing, duplicate and coverage checks and never extracts members.

The supported numeric ELF gates are GLIBC, GLIBCXX and CXXABI. Other recorded nodes are evidence, not an assertion that all ABI families were evaluated. Sectionless dynamic ELF and extended numbering, compressed runtime metadata, FAT64 and unrecognized machine formats are outside version 0.1. Recognizable unsupported runtime content prevents a clean coverage result.

These parsers are for the publisher's own build artifacts, with resource bounds. They are not an isolation boundary for adversarial malware. No claim is made about kernel or CPU features, transitive dependency resolution, `dlopen`, library ABI correctness or target-system execution.
