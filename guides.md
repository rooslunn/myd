[ ] remove temp files after combine
[x] download audio and video parallel
[ ] progress of download
[ ] set default client type (ios, android, web)

### How to min binary

A stripped Go binary of ~14 MB is very normal. Go’s runtime + garbage collector + scheduler + standard library all get statically linked in, so the baseline size rarely goes below ~10–12 MB for anything non-trivial.

Here are some more advanced tricks you can try if you really want to squeeze it further:

#### 1. Use tinygo (for small binaries)

tinygo is a Go compiler built on LLVM that’s optimized for size (used for IoT / WASM).

```tinygo build -o myapp main.go```

- Binaries can be as small as 1–2 MB.
- ⚠Limitation: not all Go standard library features are supported (especially reflect, net/http, etc.).

#### 2. Avoid importing heavy packages

Some stdlib packages (like net/http, crypto/x509, encoding/json) drag in a lot of code.

- For example, switching from encoding/json → json-iterator/go might trim a bit.
- Avoid unnecessary TLS/certs if you don’t need them (they pull in large root CA data).

#### 3. Split functionality into plugins or dynamic libs

Instead of shipping one monolithic binary, you can:

- Build your main app minimal
- Load extra features via .so plugins (with -buildmode=plugin).

Not always practical, but sometimes reduces the "core" size.

#### 5. Compress with UPX (you tried, right?)

UPX usually takes 14 MB → 4–6 MB.

```upx --best --lzma myapp```

#### 6. Compare with go tool nm

You can inspect what symbols eat space:

```go tool nm myapp | less```

That helps spot unused packages creeping in.