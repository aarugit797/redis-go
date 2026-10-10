package main

import "math/rand"

const (
	skiplistMaxLevel = 32
	skiplistP        = 0.25
)

type slLevel struct {
	forward *slNode
	span    int
}

type slNode struct {
	member   string
	score    float64
	backward *slNode
	level    []slLevel
}

type SkipList struct {
	header *slNode
	tail   *slNode
	length int
	level  int
}

func NewSkipList() *SkipList {
	return &SkipList{header: &slNode{level: make([]slLevel, skiplistMaxLevel)}, level: 1}
}

func randomLevel() int {
	lvl := 1
	for lvl < skiplistMaxLevel && rand.Float64() < skiplistP {
		lvl++
	}
	return lvl
}

func before(n *slNode, score float64, member string) bool {
	return n.score < score || (n.score == score && n.member < member)
}

func (sl *SkipList) Insert(member string, score float64) {
	var update [skiplistMaxLevel]*slNode
	var rank [skiplistMaxLevel]int

	x := sl.header
	for i := sl.level - 1; i >= 0; i-- {
		if i < sl.level-1 {
			rank[i] = rank[i+1]
		}
		for x.level[i].forward != nil && before(x.level[i].forward, score, member) {
			rank[i] += x.level[i].span
			x = x.level[i].forward
		}
		update[i] = x
	}

	lvl := randomLevel()
	if lvl > sl.level {
		for i := sl.level; i < lvl; i++ {
			rank[i] = 0
			update[i] = sl.header
			update[i].level[i].span = sl.length
		}
		sl.level = lvl
	}

	x = &slNode{member: member, score: score, level: make([]slLevel, lvl)}
	for i := 0; i < lvl; i++ {
		x.level[i].forward = update[i].level[i].forward
		update[i].level[i].forward = x

		x.level[i].span = update[i].level[i].span - (rank[0] - rank[i])
		update[i].level[i].span = (rank[0] - rank[i]) + 1
	}

	for i := lvl; i < sl.level; i++ {
		update[i].level[i].span++
	}

	if update[0] != sl.header {
		x.backward = update[0]
	}
	if x.level[0].forward != nil {
		x.level[0].forward.backward = x
	} else {
		sl.tail = x
	}
	sl.length++
}

func (sl *SkipList) Delete(member string, score float64) bool {
	var update [skiplistMaxLevel]*slNode
	x := sl.header
	for i := sl.level - 1; i >= 0; i-- {
		for x.level[i].forward != nil && before(x.level[i].forward, score, member) {
			x = x.level[i].forward
		}
		update[i] = x
	}
	x = x.level[0].forward
	if x == nil || x.score != score || x.member != member {
		return false
	}
	for i := 0; i < sl.level; i++ {
		if update[i].level[i].forward == x {
			update[i].level[i].span += x.level[i].span - 1
			update[i].level[i].forward = x.level[i].forward
		} else {
			update[i].level[i].span--
		}
	}
	if x.level[0].forward != nil {
		x.level[0].forward.backward = x.backward
	} else {
		sl.tail = x.backward
	}
	for sl.level > 1 && sl.header.level[sl.level-1].forward == nil {
		sl.level--
	}
	sl.length--
	return true
}

func (sl *SkipList) Rank(member string, score float64) int {
	rank := 0
	x := sl.header
	for i := sl.level - 1; i >= 0; i-- {
		for x.level[i].forward != nil &&
			(before(x.level[i].forward, score, member) ||
				(x.level[i].forward.score == score && x.level[i].forward.member == member)) {
			rank += x.level[i].span
			x = x.level[i].forward
		}
		if x != sl.header && x.member == member && x.score == score {
			return rank
		}
	}
	return 0
}

func (sl *SkipList) NodeAtRank(rank int) *slNode {
	traversed := 0
	x := sl.header
	for i := sl.level - 1; i >= 0; i-- {
		for x.level[i].forward != nil && traversed+x.level[i].span <= rank {
			traversed += x.level[i].span
			x = x.level[i].forward
		}
		if traversed == rank {
			return x
		}
	}
	return nil
}

func (sl *SkipList) FirstAtLeast(min float64) *slNode {
	x := sl.header
	for i := sl.level - 1; i >= 0; i-- {
		for x.level[i].forward != nil && x.level[i].forward.score < min {
			x = x.level[i].forward
		}
	}
	return x.level[0].forward
}
