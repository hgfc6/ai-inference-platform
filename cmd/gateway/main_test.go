package main

import (
	"os"
	"testing"
)

func TestEnvPositiveIntOrDefault(t *testing.T) {
	const key = "MAX_INFLIGHT_REQUESTS_TEST"
	t.Setenv(key, "")

	value, err := envPositiveIntOrDefault(key, 8)
	if err != nil {
		t.Fatalf("读取默认值失败: %v", err)
	}
	if value != 8 {
		t.Fatalf("默认值 = %d, want 8", value)
	}

	if err := os.Setenv(key, "2"); err != nil {
		t.Fatalf("设置测试环境变量失败: %v", err)
	}
	value, err = envPositiveIntOrDefault(key, 8)
	if err != nil {
		t.Fatalf("读取有效配置失败: %v", err)
	}
	if value != 2 {
		t.Fatalf("配置值 = %d, want 2", value)
	}
}

func TestEnvPositiveIntOrDefaultRejectsInvalidValue(t *testing.T) {
	const key = "MAX_INFLIGHT_REQUESTS_TEST"
	for _, value := range []string{"0", "-1", "invalid"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv(key, value)
			if _, err := envPositiveIntOrDefault(key, 8); err == nil {
				t.Fatalf("配置值 %q 未返回错误", value)
			}
		})
	}
}
