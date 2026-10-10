package main

type Hash struct {
	small   []string
	big     *Dict[string]
	payload int
}

func NewHash() *Hash { return &Hash{} }

func (h *Hash) Encoding() string {
	if h.big != nil {
		return "hashtable"
	}
	return "listpack"
}

func (h *Hash) Len() int {
	if h.big != nil {
		return h.big.Len()
	}
	return len(h.small) / 2
}

func (h *Hash) Mem() int {
	if h.big != nil {
		return 64 + 56*h.Len() + h.payload
	}
	return 7 + 4*h.Len() + h.payload
}

func (h *Hash) indexOf(field string) int {
	for i := 0; i < len(h.small); i += 2 {
		if h.small[i] == field {
			return i
		}
	}
	return -1
}

func (h *Hash) Set(field, value string) bool {
	if h.big == nil {
		i := h.indexOf(field)
		tooLong := len(field) > hashMaxListpackValue || len(value) > hashMaxListpackValue
		tooMany := i < 0 && h.Len()+1 > hashMaxListpackEntries
		if !tooLong && !tooMany {
			if i >= 0 {
				h.payload += len(value) - len(h.small[i+1])
				h.small[i+1] = value
				return false
			}
			h.small = append(h.small, field, value)
			h.payload += len(field) + len(value)
			return true
		}
		h.convert()
	}
	old, existed := h.big.Get(field)
	h.big.Set(field, value)
	if existed {
		h.payload += len(value) - len(old)
	} else {
		h.payload += len(field) + len(value)
	}
	return !existed
}

func (h *Hash) convert() {
	h.big = NewDict[string]()
	for i := 0; i < len(h.small); i += 2 {
		h.big.Set(h.small[i], h.small[i+1])
	}
	h.small = nil
}

func (h *Hash) Get(field string) (string, bool) {
	if h.big != nil {
		return h.big.Get(field)
	}
	if i := h.indexOf(field); i >= 0 {
		return h.small[i+1], true
	}
	return "", false
}

func (h *Hash) Delete(field string) bool {
	if h.big != nil {
		old, ok := h.big.Get(field)
		if !ok {
			return false
		}
		h.big.Delete(field)
		h.payload -= len(field) + len(old)
		return true
	}
	i := h.indexOf(field)
	if i < 0 {
		return false
	}
	h.payload -= len(field) + len(h.small[i+1])
	h.small = append(h.small[:i], h.small[i+2:]...)
	return true
}

func (h *Hash) All() []string {
	if h.big == nil {
		out := make([]string, len(h.small))
		copy(out, h.small)
		return out
	}
	out := make([]string, 0, h.big.Len()*2)
	h.big.ForEach(func(field, value string) {
		out = append(out, field, value)
	})
	return out
}
