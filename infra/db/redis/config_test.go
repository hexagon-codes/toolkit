package redis

import (
	"reflect"
	"testing"

	"github.com/hexagon-codes/toolkit/infra/redisconn"
)

var (
	_ redisconn.Config              = Config{}
	_ Config                        = redisconn.Config{}
	_ redisconn.Mode                = ModeSingle
	_ Mode                          = redisconn.ModeSingle
	_ redisconn.Credentials         = Credentials{}
	_ Credentials                   = redisconn.Credentials{}
	_ redisconn.CredentialsProvider = CredentialsProvider(nil)
	_ CredentialsProvider           = redisconn.CredentialsProvider(nil)
)

func TestDefaultConfigDelegatesToRedisconn(t *testing.T) {
	addrs := []string{"redis-1:6379", "redis-2:6379"}
	got := DefaultConfig(ModeCluster, addrs...)
	want := redisconn.DefaultConfig(redisconn.ModeCluster, addrs...)

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DefaultConfig() = %+v, want canonical config %+v", got, want)
	}

	addrs[0] = "mutated:6379"
	if got.Addrs[0] != "redis-1:6379" {
		t.Fatal("DefaultConfig retained the caller's address slice")
	}
}

func TestModeConstantsDelegateToRedisconn(t *testing.T) {
	tests := []struct {
		name string
		got  Mode
		want redisconn.Mode
	}{
		{name: "single", got: ModeSingle, want: redisconn.ModeSingle},
		{name: "cluster", got: ModeCluster, want: redisconn.ModeCluster},
		{name: "sentinel", got: ModeSentinel, want: redisconn.ModeSentinel},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.got != test.want {
				t.Fatalf("mode = %q, want %q", test.got, test.want)
			}
		})
	}
}

func TestValidationErrorsDelegateToRedisconn(t *testing.T) {
	if ErrInvalidContext != redisconn.ErrInvalidContext {
		t.Fatal("ErrInvalidContext does not delegate to redisconn")
	}
	if ErrInvalidConfig != redisconn.ErrInvalidConfig {
		t.Fatal("ErrInvalidConfig does not delegate to redisconn")
	}
	if ErrInvalidCredentials != redisconn.ErrInvalidCredentials {
		t.Fatal("ErrInvalidCredentials does not delegate to redisconn")
	}
	if ErrCredentialsProvider != redisconn.ErrCredentialsProvider {
		t.Fatal("ErrCredentialsProvider does not delegate to redisconn")
	}
}
