# pkg/keccak — SIMD Keccak-256 binaries

This package wraps the SIMD-accelerated, legacy Keccak-256 (Ethereum-compatible,
0x01 padding suffix — **not** FIPS 202 SHA3-256) primitives produced by
[XKCP](https://github.com/XKCP/XKCP). The actual permutation code is shipped as
two pre-linked, relocation-free `.syso` blobs that the Go linker pulls in
directly — no CGO, no toolchain dependency at `go build` time:

- `keccak_times4_linux_amd64.syso` — AVX2, 4-way parallel
- `keccak_times8_linux_amd64.syso` — AVX-512, 8-way parallel

The `.syso` files are checked in and verified against `CHECKSUM` by
`TestSysoChecksums` in `keccak_test.go`, so bee builds reproducibly without
the XKCP toolchain. Re-build the blobs only when XKCP itself changes (or when
porting to a new platform).

## Rebuilding the syso files

The build lives in our XKCP fork — the upstream repo does not carry the
Go bindings out of the box.

### 1. Check out the fork

```bash
git clone https://github.com/ethersphere/XKCP.git ~/repos/XKCP
cd ~/repos/XKCP
git submodule update --init
```

(If you already have a clone at `~/repos/XKCP`, just `git pull`.)

### 2. Install the build dependencies

The build script needs a system C toolchain, `xsltproc` (XKCP's own build
system uses XSLT for Makefile generation) and `c2goasm` (the Go assembly
generator):

```bash
# Debian/Ubuntu
sudo apt-get install build-essential xsltproc

# Arch / Manjaro
sudo pacman -S base-devel libxslt

# macOS
brew install xsltproc

# c2goasm (Go-side dependency, all platforms)
go install github.com/minio/c2goasm@latest
which c2goasm   # confirm it is in PATH
```

### 3. Run the build

From the root of the XKCP checkout:

```bash
./build_go_asm.sh
```

This script (described in detail at the top of the file) does the work in
six steps: builds the XKCP AVX2/AVX-512 libraries, compiles C wrappers that
adapt the XKCP API to a Go-friendly layout, fully links each combined object
to remove every relocation, re-wraps the result as an ET_REL ELF, and emits
both `.syso` blobs and the matching Plan 9 assembly glue under
`~/repos/XKCP/go_keccak/`.

### 4. Copy the artifacts into bee

The build script writes files named `keccak_times4_amd64.syso` /
`keccak_times8_amd64.syso`. We rename them on the way in so Go's build
constraints scope them to `linux/amd64` (`!linux` builds must not link these
blobs):

```bash
KECCAK=/home/acud/go/src/github.com/ethersphere/bee/pkg/keccak
cp ~/repos/XKCP/go_keccak/keccak_times4_amd64.syso \
   "$KECCAK/keccak_times4_linux_amd64.syso"
cp ~/repos/XKCP/go_keccak/keccak_times8_amd64.syso \
   "$KECCAK/keccak_times8_linux_amd64.syso"
```

The Plan 9 assembly stubs (`keccak_times{4,8}_linux_amd64.s`) and the Go
wrappers are hand-maintained in this directory; they should not need
re-generation unless the XKCP entry-symbol names change.

### 5. Refresh CHECKSUM and run the guard test

The `CHECKSUM` file in this directory pins the SHA-256 of each `.syso`. Update
it whenever the blobs are regenerated:

```bash
cd "$KECCAK"
sha256sum keccak_times4_linux_amd64.syso keccak_times8_linux_amd64.syso > CHECKSUM
go test ./...
```

`TestSysoChecksums` parses `CHECKSUM`, recomputes the digests of the on-disk
`.syso` files, and fails the build if either drifts — that's the CI tripwire
that catches an accidental rebuild or a corrupted check-in.
