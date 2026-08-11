package poolx

// panicHandlerSnapshot 是可通过原子指针发布的不可变处理器快照。
type panicHandlerSnapshot struct {
	handle func(any)
}

func newPanicHandlerSnapshot(handler func(any)) *panicHandlerSnapshot {
	if handler == nil {
		return nil
	}
	return &panicHandlerSnapshot{handle: handler}
}
