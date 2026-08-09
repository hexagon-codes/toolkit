// Package asynq provides a lifecycle-managed task queue based on
// github.com/hibiken/asynq.
//
// Redis connection behavior is defined exclusively by redisconn.Config. The
// topology mode is explicit: a one-seed Cluster remains a Cluster, and
// Sentinel has independent data-node and Sentinel credentials. Static
// authentication is either disabled or a complete username/password ACL pair;
// password-only configuration is intentionally rejected by project policy.
//
// A Manager verifies Redis connectivity before construction succeeds:
//
//	redisConfig := redisconn.DefaultConfig(redisconn.ModeSingle, "redis.internal:6379")
//	redisConfig.DataCredentials = redisconn.Credentials{
//		Username: "queue-worker",
//		Password: password,
//	}
//	config := queue.DefaultConfig(redisConfig)
//	config.QueuePrefix = "prod:"
//
//	startupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
//	manager, err := queue.NewManager(startupCtx, config)
//	cancel()
//	if err != nil {
//		return err
//	}
//	defer manager.Stop()
//
//	manager.RegisterHandler("email:send", handleEmail)
//	if err := manager.Start(ctx); err != nil {
//		return err
//	}
//	_, err = manager.EnqueueTask(
//		ctx,
//		"email:send",
//		payload,
//		hibasynq.Queue(queue.QueueHigh),
//	)
//
// Start reports worker and scheduler startup errors synchronously. Stop is
// idempotent and closes every Redis resource owned by the Manager.
// Queue options passed to Manager APIs are unnamespaced base names. Manager
// applies QueuePrefix, validates ownership, and supplies the configured
// default when Queue is omitted. QueueName is reserved for native Inspector
// integrations that require the resolved Redis queue name.
//
// Polling and migration coordination use token-based Redis leases. Redis and
// authentication failures fail closed; Refresh and Release use compare-token
// Lua scripts so a stale owner cannot modify a replacement owner's lock.
package asynq
