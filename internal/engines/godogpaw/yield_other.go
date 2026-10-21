//go:build !(js && wasm)

package godogpaw

// MaybeYield is a no-op on native builds.
func MaybeYield() {}
