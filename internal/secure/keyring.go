package secure

import (
	"errors"
	"fmt"

	"github.com/zalando/go-keyring"
)

// ServiceName 是 Keychain 中统一的服务名（品牌短名 np）。
// 旧版本使用 "nomad-manager" 作为 service；历史 token 不会自动迁移，
// 升级后需在编辑集群时重新录入（MVP 阶段接受，见 review 建议）。
const ServiceName = "np"

var (
	ErrTokenNotFound = errors.New("token not found")
)

// Keyring 抽象，便于测试注入 mock。
type Keyring interface {
	SaveToken(clusterID, token string) error
	GetToken(clusterID string) (string, error)
	DeleteToken(clusterID string) error
}

// OSKeyring 使用系统 Keychain：macOS Keychain / Windows Credential Manager / Linux Secret Service。
type OSKeyring struct {
	service string
}

// New 返回基于系统 Keychain 的实现。
func New() *OSKeyring {
	return &OSKeyring{service: ServiceName}
}

// SaveToken 保存集群 token 到系统 Keychain。
func (k *OSKeyring) SaveToken(clusterID, token string) error {
	if clusterID == "" || token == "" {
		return errors.New("clusterID and token are required")
	}
	if err := keyring.Set(k.service, clusterID, token); err != nil {
		return fmt.Errorf("keyring set: %w", err)
	}
	return nil
}

// GetToken 从系统 Keychain 读取集群 token。
func (k *OSKeyring) GetToken(clusterID string) (string, error) {
	if clusterID == "" {
		return "", errors.New("clusterID is required")
	}
	token, err := keyring.Get(k.service, clusterID)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return "", ErrTokenNotFound
		}
		return "", fmt.Errorf("keyring get: %w", err)
	}
	return token, nil
}

// DeleteToken 从系统 Keychain 删除集群 token。
func (k *OSKeyring) DeleteToken(clusterID string) error {
	if clusterID == "" {
		return errors.New("clusterID is required")
	}
	if err := keyring.Delete(k.service, clusterID); err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return ErrTokenNotFound
		}
		return fmt.Errorf("keyring delete: %w", err)
	}
	return nil
}
