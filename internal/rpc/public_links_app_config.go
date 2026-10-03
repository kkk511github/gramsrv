package rpc

import (
	"encoding/json"
	"fmt"
	"hash/crc32"

	"telesrv/internal/domain"
)

func publicLinksAppConfig(cfg domain.AppConfig, prefix string) (domain.AppConfig, error) {
	values := make(map[string]json.RawMessage)
	if err := json.Unmarshal(cfg.JSON, &values); err != nil {
		return cfg, err
	}
	if values == nil {
		return cfg, fmt.Errorf("app config must be an object")
	}
	values["safelink_public_link_prefix"], _ = json.Marshal(prefix)
	body, err := json.Marshal(values)
	if err != nil {
		return cfg, err
	}
	hash := int(crc32.ChecksumIEEE(body) & 0x7fffffff)
	if hash == 0 {
		hash = 1
	}
	cfg.JSON, cfg.Hash = body, hash
	return cfg, nil
}
