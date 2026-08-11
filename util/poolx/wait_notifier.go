package poolx

import "sync"

// waitNotifier 在同一把状态锁下登记和唤醒等待者，避免条件检查与休眠之间丢失通知。
type waitNotifier struct {
	nextID  uint64
	waiters map[uint64]chan struct{}
}

func newWaitNotifier() *waitNotifier {
	return &waitNotifier{waiters: make(map[uint64]chan struct{})}
}

// waitLocked 在调用方持锁时登记等待者，返回前重新持有该锁。
func (n *waitNotifier) waitLocked(lock sync.Locker, done <-chan struct{}) {
	nextID := n.nextID
	n.nextID++
	notification := make(chan struct{})
	n.waiters[nextID] = notification

	lock.Unlock()
	if done == nil {
		<-notification
	} else {
		select {
		case <-notification:
		case <-done:
		}
	}
	lock.Lock()
	delete(n.waiters, nextID)
}

// signalLocked 唤醒一个已经登记的等待者。
func (n *waitNotifier) signalLocked() {
	for id, notification := range n.waiters {
		delete(n.waiters, id)
		close(notification)
		return
	}
}

// broadcastLocked 唤醒全部已经登记的等待者。
func (n *waitNotifier) broadcastLocked() {
	for id, notification := range n.waiters {
		delete(n.waiters, id)
		close(notification)
	}
}
