# godogpaw (vendored)

Source: https://github.com/hmgle/godogpaw — MIT License, see `LICENSE`.

Vendored because the original package writes UCI info lines directly to
`os.Stdout` and uses package-level search tables. The local copy keeps the
engine logic but adds:

- `SearchWithProgress`, which reports each completed iterative-deepening level
  through a callback instead of printing.
- cancellation of the internal time-limit timer when a search returns early.
- package renamed from `engine` to `godogpaw` for a clearer local import path.

The engine's search globals are serialized by a mutex in
`internal/games/xiangqi/native.go`.
