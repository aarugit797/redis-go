=== START HERE: RE-ORIENT ME FIRST ===
It has been a while since I last worked on this project, so before teaching anything new, do this:

1. Give me a short recap of where I am — what the project is, why I'm building it, what's working right now, and what the immediate next step is. Keep it tight, not a wall of text.

2. Then quiz me on what I've already learned, BEFORE re-explaining any of it. Ask me the key questions from the concepts listed below — networking/TCP, RESP and why length-prefixing, goroutines and why they matter, hash tables and buckets/collisions, mutexes and why naive check-then-set fails, and what CAS is. Ask a few at a time, not all at once, and wait for my answers.

3. Based on how I answer, decide what to re-teach. If I answer something well, move on — don't re-explain what I clearly still know. If I'm shaky or wrong on something, go back and re-teach that specific concept properly from scratch (concept first, plain language, then technical), because everything ahead builds on these.

4. Only once I'm solid on the fundamentals below, continue from the EXACT NEXT STEP section at the bottom.

Also: do this same re-orientation whenever I come back after a gap, or whenever I ask you to "revise" — quiz first, then fill only the actual gaps.

CONTEXT: WHERE I AM IN THE REDIS-FROM-SCRATCH PROJECT

=== PROJECT PLAN (already decided, don't re-plan) ===
Language: Go (chosen deliberately — see reasoning below)
Port: 6380 (not 6379, to avoid conflict if real Redis is ever running)
Testing: using redis-cli as a known-good RESP client (installed redis-tools only, NOT running redis-server)

9 PHASES:
1. TCP server + RESP protocol parsing  ← CURRENTLY HERE (done)
2. Core key-value store + incremental rehashing
3. TTL/expiry (passive + active expiry)
4. More data types + skip lists for sorted sets
5. Persistence — AOF and RDB
6. Eviction policies (approximate LRU)
7. Pub/Sub
8. Single-threaded event loop + benchmark vs naive locked multithreaded version ← HIGHEST VALUE PHASE
9. Benchmarking + writeup

The 6 questions I must be able to answer cold by the end:
1. Why RESP instead of JSON
2. Why incremental rehashing instead of resizing all at once
3. Why AOF vs RDB is a real tradeoff
4. Why skip list for sorted sets instead of balanced tree
5. Why Redis's LRU is approximate, not exact
6. Why single-threaded execution is a deliberate performance choice, not a limitation

=== WHY GO, NOT PYTHON (already reasoned, don't re-litigate) ===
- Phase 8 needs REAL parallel execution to honestly demonstrate lock contention. Python's GIL means only one thread executes Python code at a time, so a "naive multithreaded" Python version would never show true lock contention — the benchmark would measure GIL overhead, not the real tradeoff Redis's designers faced.
- Python's async is "one worker who never idles" (concurrency), not "many workers truly at once" (parallelism). Redis's actual design is closer to async — so Python can't demonstrate the PROBLEM we're trying to prove exists.
- Go compiles to native code → honest benchmark numbers, not interpreter overhead noise.
- Not C/Rust because time spent fighting memory management/borrow checker isn't proportional payoff.

=== CONCEPTS ALREADY TAUGHT AND UNDERSTOOD ===
Networking foundation:
- Networks only ship raw bytes; nothing about reliability/order is free
- Packets can be lost, arrive out of order, duplicated, corrupted
- TCP solves this: reliable delivery (acknowledgments + resends), ordered delivery, connection via three-way handshake
- UDP = no guarantees, faster; Redis uses TCP because a lost SET is unacceptable
- A "connection" = shared bookkeeping state on both ends tracking the conversation
- Ports = numbers 0-65535, so the OS knows which program incoming data belongs to
- Sockets = the OS-level object representing a network endpoint; listening socket (waits for new connections) vs connection socket (one specific client conversation)
- Client-server model = who initiates, not who can talk after

Go language basics (taught with C++/Python comparisons):
- package main + func main() = entry point, like C++'s int main()
- Compiled + statically typed (like C++), not interpreted/dynamic (like Python)
- NO exceptions — functions return (result, error) and you check err explicitly every time
- := short declaration with type inference (like C++ auto)
- nil = null/None
- defer = run this when function returns, guaranteed cleanup (like C++ RAII / Python finally)
- Parameters written as `name Type` (reversed from C++)
- Pointers with * (same as C++)
- Slices []string = dynamic array (like std::vector / Python list, but typed)
- make() to allocate slices/maps
- _ to discard a return value you don't need
- Methods: func (s *Store) MethodName() — the (s *Store) is like C++ member function, s is like this/self
- Goroutines: `go someFunc()` launches independent concurrent execution. Lighter than OS threads (few KB stack vs MB). Go runtime schedules many goroutines onto fewer OS threads, which OS maps onto real cores → TRUE parallelism possible.

Why goroutines matter (understood deeply):
- Without `go`, handleConnection blocks the accept loop → server can only serve ONE client at a time, Client B waits for Client A to fully finish
- With `go`, the accept loop immediately continues → many clients handled independently, each blocking only on their own connection

RESP protocol (understood deeply):
- TCP delivers a STREAM of bytes, not messages. Commands can arrive glued together or split across reads.
- So we need a protocol = agreed rulebook for where things start/end
- Core idea: ANNOUNCE HOW MUCH IS COMING, THEN SEND IT
- Symbols: + simple string, - error, : integer, $ bulk string (length-prefixed), * array (count-prefixed)
- \r\n = "end of this piece" marker
- Example: SET x 1 becomes *3\r\n$3\r\nSET\r\n$1\r\nx\r\n$1\r\n1\r\n
- WHY length-prefixes instead of separators like spaces: the DATA ITSELF could contain spaces or even \r\n. By announcing exact length, you just count out that many bytes regardless of content — can't be fooled.
- WHY RESP instead of JSON: minimal parsing overhead, less allocation, faster than JSON parsing

Buffered reading (understood):
- Reading from a socket = a system call, which crosses from your program into the OS kernel — relatively expensive
- Reading one byte at a time = enormous number of expensive syscalls
- bufio.Reader grabs a chunk (e.g. 4KB) in one syscall into in-memory buffer, then parsing pulls from fast memory, refilling only when dry

Hash tables / maps (understood):
- Naive alternative = one long list, GET means scanning everything = slow
- Hash table: array of numbered "buckets" (boxes). Hash function converts key → number → bucket index. Jump DIRECTLY there, no searching.
- Hash function must be deterministic (same input → same output always) but spread keys unpredictably
- COLLISIONS: two different keys hashing to the same bucket. Inevitable (more keys than buckets, or just chance).
- So each bucket holds a small group of items, not one. Go's real map holds 8 per bucket before chaining an overflow bucket.
- So O(1) is AVERAGE case, assuming good distribution. Bad hash = degrades toward scanning a list.
- Growth: when too full (load factor), array must grow and everything redistributes. Go's map does this incrementally internally.
- NOTE: currently using Go's built-in map (someone else's hash table). Open decision: later write our own hash table for maximum depth + to implement incremental rehashing ourselves visibly.

Locks / concurrency (understood deeply, down to hardware):
- RACE CONDITION: two threads on two cores doing read-modify-write on same data → last writer wins, other update silently vanishes, or outright corruption
- MUTEX (mutual exclusion) = only one thread inside this section at a time
- WHY a naive `if locked == false { locked = true }` FAILS: that's TWO steps (read, then write). Two threads can both read false in the gap before either writes → both think they hold the lock. Can't fix with another lock (circular).
- ATOMIC = indivisible, no in-between state visible to other cores
- COMPARE-AND-SWAP (CAS) = one atomic hardware instruction: "if this memory location currently equals X, replace with Y, and tell me if it worked" — check and set glued together, no gap
- HOW HARDWARE GUARANTEES IT: cache line (typically 64 bytes, the unit moved between core cache and main memory). Executing core asserts exclusive ownership of that cache line via cache coherency protocol (MESI), forcing other cores to invalidate their copies and blocking their access until CAS completes.
- GO'S MUTEX INTERNALS:
  * Fast path (uncontended): one CAS attempt succeeds instantly, few nanoseconds, no OS involvement
  * Contended: first SPINS (retries CAS in tight loop) hoping lock frees fast
  * If spinning fails: goroutine gets PARKED — runtime tells scheduler it can't proceed, OS thread freed for other work. Later must be WOKEN and rescheduled, possibly on a different core (losing CPU cache warmth). This is the expensive path.
- THIS IS THE PHASE 8 POINT: contention cost has nothing to do with SET/GET logic itself — it's purely the physical cost of coordinating exclusive access to shared memory across cores. Redis sidesteps this ENTIRE category by never letting two things touch data simultaneously.

=== CODE WRITTEN SO FAR (main.go, working and tested) ===

package main

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
)

func main() {
	listener, err := net.Listen("tcp", ":6380")
	if err != nil {
		panic(err)
	}
	defer listener.Close()
	fmt.Println("listening on :6380")

	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("accept error:", err)
			continue
		}
		go handleConnection(conn)
	}
}

func handleConnection(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)

	for {
		cmd, err := parseRESP(reader)
		if err != nil {
			fmt.Println("connection closed or parse error:", err)
			return
		}
		fmt.Println("received command:", cmd)
	}
}

func parseRESP(reader *bufio.Reader) ([]string, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}

	count, err := strconv.Atoi(strings.TrimSpace(line[1:]))
	if err != nil {
		return nil, err
	}

	args := make([]string, count)

	for i := 0; i < count; i++ {
		lengthLine, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		length, err := strconv.Atoi(strings.TrimSpace(lengthLine[1:]))
		if err != nil {
			return nil, err
		}

		buf := make([]byte, length+2)
		_, err = io.ReadFull(reader, buf)
		if err != nil {
			return nil, err
		}
		args[i] = string(buf[:length])
	}

	return args, nil
}

STATUS: This compiles and works. redis-cli -p 6380 connects, typing SET x 1 prints "received command: [SET x 1]" on the server. Client hangs waiting for a reply because we don't send responses yet.

=== EXACT NEXT STEP ===
Was mid-way through introducing the Store type (shared map + mutex) and RESP response writing. The Store code was shown but NOT yet wired into handleConnection, and NOT yet compiled/tested:

type Store struct {
	mu   sync.Mutex
	data map[string]string
}

func NewStore() *Store {
	return &Store{data: make(map[string]string)}
}

func (s *Store) Set(key, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = value
}

func (s *Store) Get(key string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, exists := s.data[key]
	return value, exists
}

Response formats still to implement:
- SET success → +OK\r\n
- GET hit → bulk string, e.g. $1\r\n1\r\n
- GET miss → $-1\r\n (null bulk string; -1 length is the reserved "no value exists" marker, meaningfully different from an empty string)

IMPORTANT FRAMING: The Store+Mutex version is DELIBERATELY the naive "before" picture — correct but costly under contention. It exists so Phase 8 has a real baseline to benchmark Redis's actual single-threaded approach against, with real numbers rather than an abstract claim.

Start by wiring Store into handleConnection, executing parsed commands, and writing RESP replies back — with full concept-first depth per the teaching instructions.