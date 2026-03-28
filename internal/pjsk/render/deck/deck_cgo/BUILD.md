# internal/pjsk/render/deck/deck_cgo — Build Guide

## Overview

```
Go (engine.go + pool.go)
  │ CGo
  ▼
deck_c_api.h / deck_c_api.cpp     ← pure-C shim we wrote
  │ C++ calls
  ▼
sekai_deck_recommend.cpp           ← upstream C++ engine
  (NeuraXmy/sekai-deck-recommend-cpp)
```

No Python runtime required after the library is built.

---

## Source Layout

This package already vendors the minimal upstream sources required to build the
native shim:

- `vendor/sekai-deck-recommend-cpp/src`
- `vendor/sekai-deck-recommend-cpp/3rdparty/json/single_include`
- runtime static data in `Haruki-Cloud/data/sekai_deck_recommend`

---

## Step 2 — Build the shared library

### Windows (MSVC, recommended)

Requires: Visual Studio 2022 + CMake ≥ 3.15

```powershell
cd internal\pjsk\render\deck\deck_cgo
cmake -B build -G "Visual Studio 17 2022" -A x64
cmake --build build --config Release
cmake --install build --config Release
```

Output: `internal/pjsk/render/deck/deck_cgo/lib/windows_amd64/sekai_deck_recommend_c.dll`

### Windows (MinGW/MSYS2)

```bash
cd internal/pjsk/render/deck/deck_cgo
cmake -B build -G "MinGW Makefiles" -DCMAKE_BUILD_TYPE=Release
cmake --build build
cmake --install build
```

### Linux (GCC ≥ 11)

```bash
cd internal/pjsk/render/deck/deck_cgo
cmake -B build -DCMAKE_BUILD_TYPE=Release
cmake --build build -- -j$(nproc)
cmake --install build
```

Output: `internal/pjsk/render/deck/deck_cgo/lib/linux_amd64/libsekai_deck_recommend_c.so`

### macOS (Apple Clang / Homebrew GCC)

```bash
cd internal/pjsk/render/deck/deck_cgo
cmake -B build -DCMAKE_BUILD_TYPE=Release
cmake --build build -- -j$(sysctl -n hw.logicalcpu)
cmake --install build
```

---

## Step 3 — Build Go with explicit tag

Default `Haruki-Cloud` builds intentionally do **not** compile this package.
You must opt in explicitly:

```powershell
# Windows
set CGO_ENABLED=1
go build -tags pjsk_deck_cgo -o server.exe ./cmd/server
```

```bash
# Linux / macOS
CGO_ENABLED=1 go build -tags pjsk_deck_cgo -o server ./cmd/server
```

---

## Cross-compilation note

CGo does **not** support true cross-compilation out of the box.  
Recommended CI strategy:
- **Windows**: build `sekai_deck_recommend_c.dll` on a Windows runner, commit to `lib/windows_amd64/`
- **Linux**: build `.so` on a Linux runner, commit to `lib/linux_amd64/`
- **macOS**: build `.dylib` on a macOS runner, commit to `lib/darwin_amd64/` and `lib/darwin_arm64/`

All pre-built libraries can be committed to the repository (they are small, ~2–5 MB).

---

## Disabling the CGo engine

If you do not build with `-tags pjsk_deck_cgo`, `Haruki-Cloud` keeps using the
existing `recommend/auto` fallback path and never touches this package.

Set in `configs.yaml`:
```yaml
deck_recommend:
  enabled: false
  use_local_engine: false
```
