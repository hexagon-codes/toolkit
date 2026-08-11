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
	// ErrInvalidClockSkewWait 表示最大时钟回拨等待预算无效。
	ErrInvalidClockSkewWait = errors.New("idgen: clock skew wait must not be negative")
	// ErrSequenceExhausted 表示同一毫秒的序列已耗尽且时钟未在预算内前进。
	ErrSequenceExhausted = errors.New("idgen: sequence exhausted before clock advanced")
	// ErrTimestampOutOfRange 表示时间戳无法编码进 Snowflake 的 41 位时间字段。
	ErrTimestampOutOfRange = errors.New("idgen: timestamp is outside the 41-bit range")
	// ErrUninitializedGenerator 表示生成器未通过构造器完成初始化。
	ErrUninitializedGenerator = errors.New("idgen: generator is not initialized")
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

	maxTimestampDelta int64 = 1<<timestampBits - 1

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
	clock            func() time.Time
	sleep            func(time.Duration)
}

var (
	defaultSnowflake *Snowflake
	initMu           sync.Mutex
)

// InitSnowflake 初始化默认 Snowflake 生成器
// 如果之前初始化失败，允许重新初始化；如果已初始化成功，则不再重复初始化
func InitSnowflake(workerID int64) error {
	initMu.Lock()
	defer initMu.Unlock()

	if defaultSnowflake != nil {
		return nil
	}

	sf, err := NewSnowflake(workerID)
	if err != nil {
		return err
	}
	defaultSnowflake = sf
	return nil
}

// NewSnowflake 创建 Snowflake 生成器
func NewSnowflake(workerID int64) (*Snowflake, error) {
	return NewSnowflakeWithOptions(workerID, 100*time.Millisecond)
}

// NewSnowflakeWithOptions 创建 Snowflake 生成器（可配置最大时钟回拨等待时间）
//
// maxClockSkewWait: 最大时钟回拨等待时间。当检测到时钟回拨且回拨时间 <= maxClockSkewWait 时，
// 会等待时间追上。如果回拨时间 > maxClockSkewWait，GenerateSafe 返回错误，Generate 则 panic。
// 默认为 100ms，设置为 0 表示不等待（立即报错）。
func NewSnowflakeWithOptions(workerID int64, maxClockSkewWait time.Duration) (*Snowflake, error) {
	if workerID < 0 || workerID > maxWorkerID {
		return nil, fmt.Errorf("worker ID must be between 0 and %d", maxWorkerID)
	}
	if maxClockSkewWait < 0 {
		return nil, fmt.Errorf("%w: %s", ErrInvalidClockSkewWait, maxClockSkewWait)
	}

	return &Snowflake{
		epoch:            Epoch,
		workerID:         workerID,
		sequence:         0,
		lastTimestamp:    0,
		maxClockSkewWait: maxClockSkewWait,
		clock:            time.Now,
		sleep:            time.Sleep,
	}, nil
}

// Generate 生成 Snowflake ID
func (s *Snowflake) Generate() int64 {
	id, err := s.GenerateSafe()
	if err != nil {
		panic(err)
	}
	return id
}

// GenerateSafe 生成 Snowflake ID（带错误返回）
//
// 当检测到时钟回拨且超过最大等待时间时，返回 ErrClockSkew。
func (s *Snowflake) GenerateSafe() (int64, error) {
	if s == nil {
		return 0, ErrUninitializedGenerator
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.clock == nil || s.sleep == nil {
		return 0, ErrUninitializedGenerator
	}

	timestamp := s.currentTimestamp()
	if err := s.validateTimestamp(timestamp); err != nil {
		return 0, err
	}

	if timestamp < s.lastTimestamp {
		// 时钟回拨
		skew := s.lastTimestamp - timestamp
		skewDuration := time.Duration(skew) * time.Millisecond

		// 检查是否超过最大等待时间
		if skewDuration > s.maxClockSkewWait {
			return 0, ErrClockSkew
		}

		// 持锁等待可以保持 ID 单调；等待预算保证冻结时钟不会导致永久阻塞。
		var err error
		timestamp, err = s.waitUntilAtLeast(s.lastTimestamp)
		if err != nil {
			return 0, err
		}
	}

	var nextSequence int64
	if timestamp == s.lastTimestamp {
		// 同一毫秒内，序列号递增
		nextSequence = (s.sequence + 1) & maxSequence
		if nextSequence == 0 {
			// 序列号溢出，等待下一毫秒
			var err error
			timestamp, err = s.waitNextMillis(timestamp)
			if err != nil {
				return 0, err
			}
		}
	}

	if err := s.validateTimestamp(timestamp); err != nil {
		return 0, err
	}
	// 所有等待与校验成功后再一次性提交状态，错误路径不会污染后续生成。
	s.sequence = nextSequence
	s.lastTimestamp = timestamp

	// 生成 ID
	id := ((timestamp - s.epoch) << timestampShift) |
		(s.workerID << workerIDShift) |
		nextSequence

	return id, nil
}

// currentTimestamp 获取当前时间戳（毫秒）
func (s *Snowflake) currentTimestamp() int64 {
	return s.clock().UnixMilli()
}

// waitNextMillis 等待下一毫秒
func (s *Snowflake) waitNextMillis(timestamp int64) (int64, error) {
	var waited time.Duration
	for timestamp <= s.lastTimestamp {
		if waited >= s.maxClockSkewWait {
			return 0, ErrSequenceExhausted
		}
		delay := boundedDelay(100*time.Microsecond, s.maxClockSkewWait-waited)
		s.sleep(delay)
		waited += delay
		timestamp = s.currentTimestamp()
	}
	return timestamp, nil
}

// waitUntilAtLeast 等待时钟追上目标毫秒，并严格受最大回拨预算约束。
func (s *Snowflake) waitUntilAtLeast(target int64) (int64, error) {
	var waited time.Duration
	for {
		timestamp := s.currentTimestamp()
		if timestamp >= target {
			return timestamp, nil
		}
		if waited >= s.maxClockSkewWait {
			return 0, ErrClockSkew
		}
		delay := boundedDelay(time.Millisecond, s.maxClockSkewWait-waited)
		s.sleep(delay)
		waited += delay
	}
}

func boundedDelay(preferred, remaining time.Duration) time.Duration {
	if remaining < preferred {
		return remaining
	}
	return preferred
}

func (s *Snowflake) validateTimestamp(timestamp int64) error {
	if timestamp < s.epoch || timestamp > s.epoch+maxTimestampDelta {
		return fmt.Errorf("%w: timestamp=%d epoch=%d", ErrTimestampOutOfRange, timestamp, s.epoch)
	}
	return nil
}

// SnowflakeID 使用默认生成器生成 ID
func SnowflakeID() int64 {
	initMu.Lock()
	if defaultSnowflake == nil {
		initMu.Unlock()
		// 默认使用 worker ID 1
		if err := InitSnowflake(1); err != nil {
			panic(err)
		}
	} else {
		initMu.Unlock()
	}
	return defaultSnowflake.Generate()
}
