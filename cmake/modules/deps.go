//go:build deps

// Package modules pins the C++ source modules that CMakeLists.txt resolves with
// go list -m. Nothing builds this file; importing one package from each module
// keeps the module in go.mod and its sources in go.sum through go mod tidy.
package modules

import (
	_ "github.com/aperturerobotics/abseil-cpp/absl"
	_ "github.com/aperturerobotics/protobuf/src/google/protobuf"
)
