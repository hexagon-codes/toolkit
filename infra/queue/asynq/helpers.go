package asynq

// =========================================
// 辅助函数
// =========================================

// getRedisConfigFromProvider 从配置提供者获取 Redis 配置
func getRedisConfigFromProvider() *RedisConfig {
	cp := GetConfigProvider()
	if cp == nil {
		return nil
	}

	return &RedisConfig{
		Addrs:    cp.GetRedisAddrs(),
		Password: cp.GetRedisPassword(),
		Username: cp.GetRedisUsername(),
	}
}
