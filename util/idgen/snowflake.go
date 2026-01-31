package idgen

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	// ErrClockSkew 时钟回拨错误
	// 当检测到系统时钟回拨且超过最大允许等待时间时返回此错误
	ErrClockSkew = errors.New("idgen: clock skew detected, time moved backwards")
)

const (
	// Epoch 起始时间戳 (2020-01-01 00:00:00 UTC)
	Epoch int64 = 1577836800000

	// 位分配
	timestampBits = 41 // 时间戳占 41 位
	workerIDBits  = 10 // 机器 ID 占 10 位
	sequenceBits  = 12 // 序列号占 12 位

	// 最大值
	maxWorkerID = -1 ^ (-1 << workerIDBits) // 1023
	maxSequence = -1 ^ (-1 << sequenceBits) // 4095

	// 位移
	workerIDShift  = sequenceBits
	timestampShift = sequenceBits + workerIDBits
)

// Snowflake ID 生成器
type Snowflake struct {
	mu               sync.Mutex
	epoch            int64
	workerID         int64
	sequence         int64
	lastTimestamp    int64
	maxClockSkewWait time.Duration // 最大时钟回拨等待时间
}

var (
	defaultSnowflake *Snowflake
	once             sync.Once
)

// InitSnowflake 初始化默认 Snowflake 生成器
func InitSnowflake(workerID int64) error {
	var err error
	once.Do(func() {
		defaultSnowflake, err = NewSnowflake(workerID)
	})
	return err
}

// NewSnowflake 创建 Snowflake 生成器
func NewSnowflake(workerID int64) (*Snowflake, error) {
	return NewSnowflakeWithOptions(workerID, 100*time.Millisecond)
}

// NewSnowflakeWithOptions 创建 Snowflake 生成器（可配置最大时钟回拨等待时间）
//
// maxClockSkewWait: 最大时钟回拨等待时间。当检测到时钟回拨且回拨时间 <= maxClockSkewWait 时，
// 会等待时间追上。如果回拨时间 > maxClockSkewWait，Generate 会返回错误。
// 默认为 100ms，设置为 0 表示不等待（立即报错）。
func NewSnowflakeWithOptions(workerID int64, maxClockSkewWait time.Duration) (*Snowflake, error) {
	if workerID < 0 || workerID > maxWorkerID {
		return nil, fmt.Errorf("worker ID must be between 0 and %d", maxWorkerID)
	}

	return &Snowflake{
		epoch:            Epoch,
		workerID:         workerID,
		sequence:         0,
		lastTimestamp:    0,
		maxClockSkewWait: maxClockSkewWait,
	}, nil
}

// Generate 生成 Snowflake ID
func (s *Snowflake) Generate() int64 {
	id, err := s.GenerateSafe()
	if err != nil {
		// 为保持向后兼容，失败时返回 0（调用者可使用 GenerateSafe 获取错误）
		return 0
	}
	return id
}

// GenerateSafe 生成 Snowflake ID（带错误返回）
//
// 当检测到时钟回拨且超过最大等待时间时，返回 ErrClockSkew。
func (s *Snowflake) GenerateSafe() (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	timestamp := s.currentTimestamp()

	if timestamp < s.lastTimestamp {
		// 时钟回拨
		skew := s.lastTimestamp - timestamp
		skewDuration := time.Duration(skew) * time.Millisecond

		// 检查是否超过最大等待时间
		if skewDuration > s.maxClockSkewWait {
			return 0, ErrClockSkew
		}

		// 等待直到时间追上（有限等待）
		deadline := time.Now().Add(s.maxClockSkewWait)
		for timestamp < s.lastTimestamp {
			if time.Now().After(deadline) {
				return 0, ErrClockSkew
			}
			time.Sleep(time.Millisecond)
			timestamp = s.currentTimestamp()
		}
	}

	if timestamp == s.lastTimestamp {
		// 同一毫秒内，序列号递增
		s.sequence = (s.sequence + 1) & maxSequence
		if s.sequence == 0 {
			// 序列号溢出，等待下一毫秒
			timestamp = s.waitNextMillis(timestamp)
		}
	} else {
		// 新的毫秒，序列号重置
		s.sequence = 0
	}

	s.lastTimestamp = timestamp

	// 生成 ID
	id := ((timestamp - s.epoch) << timestampShift) |
		(s.workerID << workerIDShift) |
		s.sequence

	return id, nil
}

// currentTimestamp 获取当前时间戳（毫秒）
func (s *Snowflake) currentTimestamp() int64 {
	return time.Now().UnixMilli()
}

// waitNextMillis 等待下一毫秒
func (s *Snowflake) waitNextMillis(timestamp int64) int64 {
	for timestamp <= s.lastTimestamp {
		timestamp = s.currentTimestamp()
	}
	return timestamp
}

// SnowflakeID 使用默认生成器生成 ID
func SnowflakeID() int64 {
	if defaultSnowflake == nil {
		// 默认使用 worker ID 1
		InitSnowflake(1)
	}
	return defaultSnowflake.Generate()
}
