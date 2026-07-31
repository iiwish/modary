//go:build !linux && !darwin

package projecttool

import "fmt"

func readProcessIDForTest(string) (int, error) {
	return 0, fmt.Errorf("process-group tests are unsupported")
}

func processExistsForTest(int) bool { return false }
