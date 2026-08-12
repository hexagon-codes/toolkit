//go:build !unix && !windows

package hostprocess

import (
	"context"
	"fmt"
)

func run(_ context.Context, _ Spec, result Result) (Result, error) {
	return result, fmt.Errorf("host process lifecycle is not supported on this platform")
}
