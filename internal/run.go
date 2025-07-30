package internal

import (
	"sync"
)

type StatisticList struct {
	mu    sync.RWMutex
	items []*PingStatistic
}

func NewStatisticList(len int) *StatisticList {
	return &StatisticList{
		items: make([]*PingStatistic, len),
	}
}

func (l *StatisticList) Append(item *PingStatistic) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.items = append(l.items, item)
}

func (l *StatisticList) Get(index int) *PingStatistic {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if index >= len(l.items) {
		return nil
	}
	return l.items[index]
}

func (l *StatisticList) Len() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.items)
}

func (l *StatisticList) Remove(index int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if index < 0 || index >= len(l.items) {
		return
	}
	l.items = append(l.items[:index], l.items[index+1:]...)
}
