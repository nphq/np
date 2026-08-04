//go:build e2e

package e2e

import (
	"os"
	"testing"
)

// TestMain 在所有 e2e 测试结束后回收共享 nomad 容器。
// 不在 startNomadDevLocked 里挂 t.Cleanup：那个 t 是首个调用者的 t，
// 它结束就会把容器删掉，导致后续测试连不上。
func TestMain(m *testing.M) {
	code := m.Run()
	if shared != nil {
		stopContainer(shared.ContainerID)
	}
	os.Exit(code)
}
