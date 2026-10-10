package main

type listBlock struct {
	items      []string
	bytes      int
	prev, next *listBlock
}

func entryCost(s string) int { return len(s) + 2 }

func (b *listBlock) fits(v string) bool {
	return len(b.items) == 0 || b.bytes+entryCost(v) <= listMaxBlockBytes
}

type List struct {
	head, tail *listBlock
	length     int
	blocks     int
	bytes      int
}

func NewList() *List { return &List{} }

func (l *List) Len() int { return l.length }

func (l *List) Encoding() string {
	if l.blocks <= 1 {
		return "listpack"
	}
	return "quicklist"
}

func (l *List) Mem() int { return l.bytes + l.blocks*(7+32) }

func (l *List) PushFront(v string) {
	if l.head == nil || !l.head.fits(v) {
		b := &listBlock{next: l.head}
		if l.head != nil {
			l.head.prev = b
		} else {
			l.tail = b
		}
		l.head = b
		l.blocks++
	}
	h := l.head
	h.items = append(h.items, "")
	copy(h.items[1:], h.items)
	h.items[0] = v
	h.bytes += entryCost(v)
	l.bytes += entryCost(v)
	l.length++
}

func (l *List) PushBack(v string) {
	if l.tail == nil || !l.tail.fits(v) {
		b := &listBlock{prev: l.tail}
		if l.tail != nil {
			l.tail.next = b
		} else {
			l.head = b
		}
		l.tail = b
		l.blocks++
	}
	l.tail.items = append(l.tail.items, v)
	l.tail.bytes += entryCost(v)
	l.bytes += entryCost(v)
	l.length++
}

func (l *List) PopFront() (string, bool) {
	h := l.head
	if h == nil {
		return "", false
	}
	v := h.items[0]
	copy(h.items, h.items[1:])
	h.items[len(h.items)-1] = ""
	h.items = h.items[:len(h.items)-1]
	h.bytes -= entryCost(v)
	l.bytes -= entryCost(v)
	l.length--
	if len(h.items) == 0 {
		l.removeBlock(h)
	}
	return v, true
}

func (l *List) PopBack() (string, bool) {
	t := l.tail
	if t == nil {
		return "", false
	}
	n := len(t.items)
	v := t.items[n-1]
	t.items[n-1] = ""
	t.items = t.items[:n-1]
	t.bytes -= entryCost(v)
	l.bytes -= entryCost(v)
	l.length--
	if len(t.items) == 0 {
		l.removeBlock(t)
	}
	return v, true
}

func (l *List) removeBlock(b *listBlock) {
	if b.prev != nil {
		b.prev.next = b.next
	} else {
		l.head = b.next
	}
	if b.next != nil {
		b.next.prev = b.prev
	} else {
		l.tail = b.prev
	}
	l.blocks--
}

func (l *List) Index(i int) (string, bool) {
	if i < 0 {
		i += l.length
	}
	if i < 0 || i >= l.length {
		return "", false
	}
	for b := l.head; b != nil; b = b.next {
		if i < len(b.items) {
			return b.items[i], true
		}
		i -= len(b.items)
	}
	return "", false
}

func (l *List) Range(start, stop int) []string {
	start, stop, ok := normalizeRange(start, stop, l.length)
	if !ok {
		return nil
	}
	out := make([]string, 0, stop-start+1)
	pos := 0
	for b := l.head; b != nil && pos <= stop; b = b.next {
		n := len(b.items)
		if pos+n <= start {
			pos += n
			continue
		}
		for i, v := range b.items {
			if p := pos + i; p >= start && p <= stop {
				out = append(out, v)
			}
		}
		pos += n
	}
	return out
}

func normalizeRange(start, stop, length int) (int, int, bool) {
	if start < 0 {
		start += length
	}
	if stop < 0 {
		stop += length
	}
	if start < 0 {
		start = 0
	}
	if stop >= length {
		stop = length - 1
	}
	if start > stop || start >= length {
		return 0, 0, false
	}
	return start, stop, true
}
