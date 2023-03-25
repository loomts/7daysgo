package raft

//
// this is an outline of the API that raft must expose to
// the service (or tester). see comments below for
// each of these functions for more details.
//
// rf = Make(...)
//   create a new raft server.
// rf.Start(command interface{}) (index, term, isleader)
//   start agreement on a new log entry
// rf.GetState() (term, isLeader)
//   ask a raft for its current term, and whether it thinks it is leader
// ApplyMsg
//   each time a new entry is committed to the log, each raft peer
//   should send an ApplyMsg to the service (or tester)
//   in the same server.
//

import (
	"6.824/labgob"
	"6.824/labrpc"
	"bytes"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

// as each Raft peer becomes aware that successive log entries are
// committed, the peer should send an ApplyMsg to the service (or
// tester) on the same server, via the applyCh passed to Make(). set
// CommandValid to true to indicate that the ApplyMsg contains a newly
// committed log entry.
//
// in part 2D you'll want to send other kinds of messages (e.g.,
// snapshots) on the applyCh, but set CommandValid to false for these
// other uses.
const NULL int = -1
const (
	_ = iota
	Follower
	Candidate
	Leader
)

type ApplyMsg struct {
	CommandValid bool
	Command      interface{}
	CommandTerm  int
	CommandIndex int

	SnapshotValid bool
	Snapshot      []byte
	SnapshotTerm  int
	SnapshotIndex int
}

type Entry struct {
	Command interface{} // Command for state machine
	Term    int         // Command received Term
	Index   int
}

// A Go object implementing a single Raft peer.
type Raft struct {
	mu        sync.Mutex          // Lock to protect shared access to this peer's state
	peers     []*labrpc.ClientEnd // RPC end points of all peers
	persister *Persister          // Object to hold this peer's persisted state
	me        int                 // this peer's index into peers[]
	dead      int32               // set by Kill()

	// Persistent state
	currentTerm int     // 0...
	votedFor    int     // null if none
	log         []Entry // index 1...

	// Volatile state
	commitIndex    int // most of the server has been replicated and durable, 0...
	lastApplied    int // highest entry applied to state machine, 0...
	state          int
	heartbeatTimer *time.Timer
	electionTimer  *time.Timer
	applyCh        chan ApplyMsg
	applyWaker     chan int
	// for leader (reinitialized after election)
	nextIndex  []int // last log index+1...
	matchIndex []int // highest log entry be replicated on server, 0..., update: PrevLogIndex+len(Entries)
}

func (rf *Raft) lastIncludedTerm() int {
	return rf.log[0].Term
}

func (rf *Raft) lastIncludedIndex() int {
	return rf.log[0].Index
}

func (rf *Raft) setLastIncludedTerm(term int) {
	rf.log[0].Term = term
}

func (rf *Raft) setLastIncludedIndex(index int) {
	rf.log[0].Index = index
}

func getHeartbeatDuration() time.Duration {
	return time.Millisecond * 50
}

func getElectionDuration() time.Duration {
	return time.Millisecond * time.Duration(rand.Int31n(150)+150)
}

func min(a, b int) int {
	if a > b {
		return b
	}
	return a
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (rf *Raft) Size() int {
	return rf.persister.RaftStateSize()
}

func (rf *Raft) getLastLogL() Entry {
	return rf.log[len(rf.log)-1]
}

func (rf *Raft) logAt(i int) Entry {
	return rf.log[i-rf.log[0].Index]
}
func (rf *Raft) HasLogAtCurrentTerm() bool {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	return rf.getLastLogL().Term == rf.currentTerm
}
func (rf *Raft) changeStateL(state int, args ...int) {
	var st string
	if state == Follower {
		rf.currentTerm, rf.votedFor = args[0], args[1]
		st = "Follower"
		if rf.state != state {
			DPrintf(dState, "S%v -> %v term%v, voteFor%v", rf.me, st, rf.currentTerm, rf.votedFor)
		}
	} else if state == Candidate {
		st = "Candidate"
		rf.currentTerm++
		rf.votedFor = rf.me
		if rf.state != state {
			DPrintf(dState, "S%v -> %v term%v, voteFor%v", rf.me, st, rf.currentTerm, rf.votedFor)
		}
	} else {
		st = "Leader"
		rf.startHeartbeatL(true)
		rf.heartbeatTimer.Reset(getHeartbeatDuration())
		if rf.state != state {
			DPrintf(dState, "S%v -> %v term%v", rf.me, st, rf.currentTerm)
		}
	}
	rf.state = state
}

func (rf *Raft) getEncodeStateL() []byte {
	w := new(bytes.Buffer)
	e := labgob.NewEncoder(w)
	e.Encode(rf.currentTerm)
	e.Encode(rf.votedFor)
	e.Encode(rf.log)
	e.Encode(rf.lastIncludedIndex())
	e.Encode(rf.lastIncludedTerm())
	return w.Bytes()
}

// return currentTerm and whether this server
// believes it is the leader.
func (rf *Raft) GetState() (int, bool) {
	var term int
	var isleader bool
	rf.mu.Lock()
	term = rf.currentTerm
	isleader = rf.state == Leader
	rf.mu.Unlock()
	return term, isleader
}

// save Raft's persistent state to stable storage,
// where it can later be retrieved after a crash and restart.
// see paper's Figure 2 for a description of what should be persistent.
func (rf *Raft) persist() {
	rf.persister.SaveRaftState(rf.getEncodeStateL())
}

// restore previously persisted state.
func (rf *Raft) readPersist(data []byte) {
	if data == nil || len(data) < 1 { // bootstrap without any state?
		return
	}
	r := bytes.NewBuffer(data)
	d := labgob.NewDecoder(r)
	var currentTerm int
	var votedFor int
	var log []Entry
	var lastIncludeIndex int
	var lastIncludeTerm int
	if d.Decode(&currentTerm) != nil ||
		d.Decode(&votedFor) != nil ||
		d.Decode(&log) != nil ||
		d.Decode(&lastIncludeIndex) != nil ||
		d.Decode(&lastIncludeTerm) != nil {
		DPrintf(dWarn, "decode err!")
	} else {
		rf.mu.Lock()
		rf.currentTerm = currentTerm
		rf.votedFor = votedFor
		rf.log = log
		rf.setLastIncludedIndex(lastIncludeIndex)
		rf.setLastIncludedTerm(lastIncludeTerm)
		rf.commitIndex = lastIncludeIndex
		rf.lastApplied = lastIncludeIndex
		rf.mu.Unlock()
	}
}

type SnapshotArgs struct {
	Term              int
	LeaderId          int
	LastIncludedIndex int
	LastIncludedTerm  int
	Snapshot          []byte
}

type SnapshotReply struct {
	Term int
}

func (rf *Raft) InstallSnapshot(args *SnapshotArgs, reply *SnapshotReply) {
	DPrintf(dSnap, "S%v receive lastTerm:%v,lastIndex:%v", rf.me, args.LastIncludedTerm, args.LastIncludedIndex)
	rf.mu.Lock()
	defer rf.mu.Unlock()
	// term higher || installed longer snapshot
	// assume that leader only will send
	reply.Term = rf.currentTerm
	if args.Term < rf.currentTerm {
		return
	}
	if args.Term > rf.currentTerm {
		rf.currentTerm, rf.votedFor = args.Term, NULL
	} else {
		rf.electionTimer.Reset(getElectionDuration())
	}
	rf.changeStateL(Follower, rf.currentTerm, rf.votedFor)
	if args.LastIncludedIndex <= rf.lastIncludedIndex() {
		return
	}
	go rf.snapshoter(&args.Snapshot, args.LastIncludedTerm, args.LastIncludedIndex)
}

func (rf *Raft) sendSnapshot(server int, args *SnapshotArgs, reply *SnapshotReply) bool {
	ok := rf.peers[server].Call("Raft.InstallSnapshot", args, reply)
	return ok
}

func (rf *Raft) foraSnapshot(peer int, args *SnapshotArgs) {
	DPrintf(dSnap, "S%v -> S%v send Snapshot", rf.me, peer)
	reply := SnapshotReply{}
	rf.mu.Lock()
	if rf.state != Leader {
		rf.mu.Unlock()
		return
	}
	rf.mu.Unlock()
	if rf.sendSnapshot(peer, args, &reply) {
		rf.mu.Lock()
		defer rf.mu.Unlock()
		DPrintf(dSnap, "S%v <- S%v receive Snapshot", rf.me, peer)
		if reply.Term > rf.currentTerm {
			rf.changeStateL(Follower, reply.Term, NULL)
			rf.persist()
		} else if args.Term == rf.currentTerm && rf.state == Leader {
			rf.nextIndex[peer] = max(rf.nextIndex[peer], args.LastIncludedIndex+1)
			rf.matchIndex[peer] = max(rf.matchIndex[peer], args.LastIncludedIndex)
			DPrintf(dSnap, "S%v next:%v, match:%v", peer, rf.nextIndex[peer], rf.matchIndex[peer])
		}
	}
}

// A service wants to switch to snapshot.  Only do so if Raft hasn't
// have more recent info since it communicate the snapshot on applyCh.
func (rf *Raft) CondInstallSnapshot(lastIncludedTerm int, lastIncludedIndex int, snapshot []byte) bool {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	if rf.commitIndex >= lastIncludedIndex {
		return false
	}
	// set the first log.term = last snapshot term
	if lastIncludedIndex < len(rf.log) && rf.logAt(lastIncludedIndex).Term == lastIncludedTerm {
		newEntries := make([]Entry, len(rf.log)-lastIncludedIndex+rf.lastIncludedIndex())
		copy(newEntries, rf.log[lastIncludedIndex-rf.lastIncludedIndex():])
		rf.log = newEntries
	} else {
		rf.log = make([]Entry, 1)
	}
	rf.setLastIncludedIndex(lastIncludedIndex)
	rf.setLastIncludedTerm(lastIncludedTerm)
	rf.commitIndex, rf.lastApplied = lastIncludedIndex, lastIncludedIndex
	rf.persister.SaveStateAndSnapshot(rf.getEncodeStateL(), snapshot)
	DPrintf(dSnap, "S%v persister snapshot,lastTerm:%v, lastIndex:%v", rf.me, lastIncludedTerm, lastIncludedIndex)
	return true
}

// the service says it has created a snapshot that has
// all info up to and including index. this means the
// service no longer needs the log through (and including)
// that index. Raft should now trim its log as much as possible.
func (rf *Raft) Snapshot(index int, snapshot []byte) {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	if rf.lastIncludedIndex() >= index {
		return
	}
	term := rf.logAt(index).Term
	rf.log = rf.log[index-rf.lastIncludedIndex():]
	rf.setLastIncludedIndex(index)
	rf.setLastIncludedTerm(term)
	rf.persister.SaveStateAndSnapshot(rf.getEncodeStateL(), snapshot)
	DPrintf(dSnap, "S%v last: %v", rf.me, index)
}

type AppendEntriesArgs struct {
	Term         int // leader's term
	LeaderId     int
	PrevLogIndex int // the index before Entries
	PrevLogTerm  int
	Entries      []Entry
	LeaderCommit int
}

type AppendEntriesReply struct {
	Term    int
	Success bool
	XTerm   int
	XIndex  int
}

func (rf *Raft) AppendEntries(args *AppendEntriesArgs, reply *AppendEntriesReply) {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	reply.XTerm, reply.XIndex = NULL, NULL
	if args.Term < rf.currentTerm {
		reply.Term, reply.Success = rf.currentTerm, false
		return
	}
	if args.Term > rf.currentTerm {
		rf.currentTerm, rf.votedFor = args.Term, NULL
	} else {
		rf.electionTimer.Reset(getElectionDuration())
	}
	rf.changeStateL(Follower, rf.currentTerm, rf.votedFor)
	// last log index is too small
	if args.PrevLogIndex > rf.getLastLogL().Index || args.PrevLogIndex < rf.lastIncludedIndex() {
		reply.Success, reply.Term = false, rf.currentTerm
		reply.XIndex = rf.getLastLogL().Index + 1 // follower's nextIndex
		return
	}
	// entry logAt prevLogIndex whose term doesn't match prevLogTerm
	DPrintf(dInfo, "S%v prevLogTerm:%v, prevLogIndex:%v,lastIncludeIndex:%v,log[prevLogIndex].Term:%v", rf.me, args.PrevLogTerm, args.PrevLogIndex, rf.lastIncludedIndex(), rf.logAt(args.PrevLogIndex).Term)
	if args.PrevLogTerm != rf.logAt(args.PrevLogIndex).Term {
		reply.Success, reply.Term = false, rf.currentTerm
		reply.XIndex, reply.XTerm = rf.commitIndex+1, rf.logAt(args.PrevLogIndex).Term
		for i := args.PrevLogIndex; i > rf.commitIndex+1; i-- {
			if reply.XTerm != rf.logAt(i-1).Term {
				reply.XIndex = i
				return
			}
		}
		DPrintf(dInfo, "S%v XIndex = %v", rf.me, rf.commitIndex+1)
		return
	}
	// log比leader短 || log比leader长并且存在不匹配 -> 截断并补上
	needReplace := rf.getLastLogL().Index <= args.PrevLogIndex+len(args.Entries)
	if len(args.Entries) > 0 {
		for i := args.PrevLogIndex + 1; i <= args.PrevLogIndex+len(args.Entries); i++ {
			// idx and term can identify a log
			if rf.getLastLogL().Index >= i && rf.logAt(i).Term != args.Entries[i-args.PrevLogIndex-1].Term {
				needReplace = true
				break
			}
		}
		if needReplace {
			rf.log = append(rf.log[:args.PrevLogIndex+1-rf.lastIncludedIndex()], args.Entries...)
		}
	}
	if args.LeaderCommit > rf.commitIndex {
		rf.commitIndex = args.LeaderCommit
		if len(args.Entries) > 0 {
			rf.commitIndex = min(rf.commitIndex, args.Entries[len(args.Entries)-1].Index)
		}
		DPrintf(dInfo, "S%v lastIndex:%v, commIndex:%v, lastApplied:%v", rf.me, rf.getLastLogL().Index, rf.commitIndex, rf.lastApplied)
		if rf.commitIndex > rf.lastApplied {
			rf.applyWaker <- 1
		}
	}
	reply.Term, reply.Success = rf.currentTerm, true
	rf.persist()
	return
}

func (rf *Raft) sendAppendEntries(server int, args *AppendEntriesArgs, reply *AppendEntriesReply) bool {
	ok := rf.peers[server].Call("Raft.AppendEntries", args, reply)
	return ok
}

func (rf *Raft) updCommitIndexL() {
	// O (peers * log) from lastIndex to commitIndex, if matchCnt > peers/2, update commitIndex
	for i := rf.getLastLogL().Index; i > rf.commitIndex; i-- {
		if rf.logAt(i).Term != rf.currentTerm {
			break
		}
		cnt := 1
		for p := range rf.peers {
			if p != rf.me && rf.matchIndex[p] >= i {
				cnt++
			}
		}
		if cnt > len(rf.peers)/2 {
			rf.commitIndex = i
			rf.applyWaker <- 1
			break
		}
	}
}

func (rf *Raft) foraHeartbeat(peer int, args *AppendEntriesArgs) {
	DPrintf(dHeart, "S%v -> S%v send heartbeat", rf.me, peer)
	reply := AppendEntriesReply{}
	rf.mu.Lock()
	if rf.state != Leader {
		rf.mu.Unlock()
		return
	}
	rf.mu.Unlock()
	if rf.sendAppendEntries(peer, args, &reply) {
		rf.mu.Lock()
		defer rf.mu.Unlock()
		if reply.Term > rf.currentTerm {
			rf.changeStateL(Follower, reply.Term, NULL)
		} else if args.Term == rf.currentTerm && rf.state == Leader {
			if reply.Success {
				rf.nextIndex[peer] = max(rf.nextIndex[peer], args.PrevLogIndex+len(args.Entries)+1)
				rf.matchIndex[peer] = max(rf.matchIndex[peer], args.PrevLogIndex+len(args.Entries))
				DPrintf(dHeart, "S%v <- S%v heartbeat matchIndex:%v", rf.me, peer, rf.matchIndex[peer])
				if len(args.Entries) > 0 && rf.getLastLogL().Term == rf.currentTerm {
					rf.updCommitIndexL()
				}
				DPrintf(dHeart, "S%v <- S%v afterCLcheck", rf.me, peer)
			} else if reply.Term != rf.currentTerm {
				DPrintf(dWarn, "S%v <- S%v invalid heartbeat", rf.me, peer)
			} else if reply.XIndex != NULL {
				DPrintf(dHeart, "S%v <- S%v heartbeat XIndex:%v, XTerm:%v", rf.me, peer, reply.XIndex, reply.XTerm)
				if reply.XTerm == NULL {
					rf.nextIndex[peer] = reply.XIndex
				} else {
					ok := false
					for i := rf.nextIndex[peer] - 1; i > rf.lastIncludedIndex() && reply.XTerm <= rf.logAt(i).Term; i-- {
						if rf.logAt(i).Term == reply.XTerm {
							ok = true
							rf.nextIndex[peer] = i + 1
							break
						}
					}
					if !ok {
						rf.nextIndex[peer] = reply.XIndex
					}
				}

			}
		}
		rf.persist()
	}
}

func (rf *Raft) startHeartbeatL(heartbeat bool) {
	for i := range rf.peers {
		if i != rf.me && (heartbeat || rf.getLastLogL().Index > rf.nextIndex[i]) {
			if rf.nextIndex[i] <= rf.lastIncludedIndex() {
				args := &SnapshotArgs{
					Term:              rf.currentTerm,
					LeaderId:          rf.me,
					Snapshot:          rf.persister.ReadSnapshot(),
					LastIncludedIndex: rf.lastIncludedIndex(),
					LastIncludedTerm:  rf.lastIncludedTerm(),
				}
				go rf.foraSnapshot(i, args)
			} else {
				prevLogIndex := rf.nextIndex[i] - 1
				prevLogTerm := rf.logAt(prevLogIndex).Term
				args := &AppendEntriesArgs{
					Term:         rf.currentTerm,
					LeaderId:     rf.me,
					PrevLogIndex: prevLogIndex,
					PrevLogTerm:  prevLogTerm,
					Entries:      make([]Entry, rf.getLastLogL().Index-prevLogIndex),
					LeaderCommit: rf.commitIndex,
				}
				// 必须要复制一遍才能免遭data race
				copy(args.Entries, rf.log[prevLogIndex+1-rf.lastIncludedIndex():])
				go rf.foraHeartbeat(i, args)
			}
		}
	}
}

// example RequestVote RPC arguments structure.
// field names must start with capital letters!
type RequestVoteArgs struct {
	Term         int
	CandidateId  int
	LastLogIndex int
	LastLogTerm  int
}

// example RequestVote RPC reply structure.
// field names must start with capital letters!
type RequestVoteReply struct {
	Term        int
	VoteGranted bool
}

// example RequestVote RPC handler.
func (rf *Raft) RequestVote(args *RequestVoteArgs, reply *RequestVoteReply) {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	defer rf.persist()
	// currTerm higher || voted
	if rf.currentTerm > args.Term || (rf.votedFor != NULL && rf.currentTerm == args.Term) {
		reply.Term, reply.VoteGranted = rf.currentTerm, false
		return
	}
	if rf.currentTerm < args.Term {
		rf.changeStateL(Follower, args.Term, NULL)
	}
	//  check log leader
	//  candidate lastLogTerm to short || (lastLogTerm=rf.lastLogTerm && candidate lastLogIndex to short)
	if rf.getLastLogL().Term > args.LastLogTerm || (rf.getLastLogL().Term == args.LastLogTerm && rf.getLastLogL().Index > args.LastLogIndex) {
		reply.Term, reply.VoteGranted = rf.currentTerm, false
		return
	}
	rf.changeStateL(Follower, args.Term, args.CandidateId)
	rf.electionTimer.Reset(getElectionDuration())
	reply.Term, reply.VoteGranted = rf.currentTerm, true
}

// example code to send a RequestVote RPC to a server.
// server is the index of the target server in rf.peers[].
// expects RPC arguments in args.
// fills in *reply with RPC reply, so caller should
// pass &reply.
// the types of the args and reply passed to Call() must be
// the same as the types of the arguments declared in the
// handler function (including whether they are pointers).
//
// The labrpc package simulates a lossy network, in which servers
// may be unreachable, and in which requests and replies may be lost.
// Call() sends a request and waits for a reply. If a reply arrives
// within a timeout interval, Call() returns true; otherwise
// Call() returns false. Thus Call() may not return for a while.
// A false return can be caused by a dead server, a live server that
// can't be reached, a lost request, or a lost reply.
//
// Call() is guaranteed to return (perhaps after a delay) *except* if the
// handler function on the server side does not return.  Thus there
// is no need to implement your own timeouts around Call().
//
// look logAt the comments in ../labrpc/labrpc.go for more details.
//
// if you're having trouble getting RPC to work, check that you've
// capitalized all field names in structs passed over RPC, and
// that the caller passes the address of the reply struct with &, not
// the struct itself.
func (rf *Raft) sendRequestVote(server int, args *RequestVoteArgs, reply *RequestVoteReply) bool {
	ok := rf.peers[server].Call("Raft.RequestVote", args, reply)
	return ok
}

func (rf *Raft) foraVote(peer int, args *RequestVoteArgs, cnt *int) {
	reply := RequestVoteReply{}
	DPrintf(dVote, "S%v -> S%v send vote", rf.me, peer)
	if rf.sendRequestVote(peer, args, &reply) {
		rf.mu.Lock()
		defer rf.mu.Unlock()
		if args.Term == rf.currentTerm && rf.state == Candidate {
			if reply.VoteGranted {
				DPrintf(dVote, "S%v <- S%v voted", rf.me, peer)
				*cnt++
				if *cnt > len(rf.peers)/2 {
					for i := range rf.peers {
						rf.nextIndex[i] = rf.getLastLogL().Index + 1
						rf.matchIndex[i] = 0
						DPrintf(dHeart, "S%v <- S%v nextIndex:%v", rf.me, i, rf.nextIndex[i])
					}
					rf.changeStateL(Leader)
				}
			} else if reply.Term > rf.currentTerm {
				DPrintf(dVote, "S%v <- S%v ~voted", rf.me, peer)
				rf.changeStateL(Follower, reply.Term, NULL)
				rf.persist()
			}
		}
	}
}

func (rf *Raft) startElectionL() {
	rf.changeStateL(Candidate)
	rf.persist()
	args := &RequestVoteArgs{
		Term:         rf.currentTerm,
		CandidateId:  rf.me,
		LastLogIndex: rf.getLastLogL().Index,
		LastLogTerm:  rf.getLastLogL().Term,
	}
	cnt := 1
	for peer := range rf.peers {
		if peer != rf.me {
			go rf.foraVote(peer, args, &cnt)
		}
	}
}

// the service using Raft (e.g. a k/v server) wants to start
// agreement on the next command to be appended to Raft's log. if this
// server isn't the leader, returns false. otherwise start the
// agreement and return immediately. there is no guarantee that this
// command will ever be committed to the Raft log, since the leader
// may fail or lose an election. even if the Raft instance has been killed,
// this function should return gracefully.
//
// the first return value is the index that the command will appear logAt
// if it's ever committed. the second return value is the current
// term. the third return value is true if this server believes it is
// the leader.
func (rf *Raft) Start(command interface{}) (int, int, bool) {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	if rf.state != Leader {
		return NULL, NULL, false
	}
	index, term := rf.getLastLogL().Index+1, rf.currentTerm
	rf.log = append(rf.log, Entry{
		Command: command,
		Term:    term,
		Index:   index,
	})
	DPrintf(dInfo, "S%v <- %vst command %v", rf.me, index, command)
	rf.persist()
	rf.startHeartbeatL(false)
	//rf.heartbeatTimer.Reset(getHeartbeatDuration())
	return index, term, true
}

// the tester doesn't halt goroutines created by Raft after each test,
// but it does call the Kill() method. your code can use killed() to
// check whether Kill() has been called. the use of atomic avoids the
// need for a lock.
//
// the issue is that long-running goroutines use memory and may chew
// up CPU time, perhaps causing later tests to fail and generating
// confusing debug output. any goroutine with a long-running loop
// should call killed() to check whether it should stop.
func (rf *Raft) Kill() {
	atomic.StoreInt32(&rf.dead, 1)
	DPrintf(dWarn, "S%v be killed", rf.me)
}

func (rf *Raft) killed() bool {
	z := atomic.LoadInt32(&rf.dead)
	return z == 1
}

// The ticker go routine starts a new election if this peer hasn't received
// heartbeats recently.
func (rf *Raft) ticker() {
	for !rf.killed() {
		// Your code here to check if a leader election should
		// be started and to randomize sleeping time using
		// time.Sleep().
		select {
		case <-rf.heartbeatTimer.C:
			rf.mu.Lock()
			if rf.state == Leader {
				rf.startHeartbeatL(true)
				rf.heartbeatTimer.Reset(getHeartbeatDuration())
				rf.electionTimer.Reset(getElectionDuration())
			}
			rf.mu.Unlock()
		case <-rf.electionTimer.C:
			rf.mu.Lock()
			rf.startElectionL()
			rf.electionTimer.Reset(getElectionDuration())
			rf.mu.Unlock()
		}
	}
}

func (rf *Raft) applier() {
	for !rf.killed() {
		select {
		case <-rf.applyWaker:
			rf.mu.Lock()
			entries := make([]Entry, rf.commitIndex-rf.lastApplied)
			copy(entries, rf.log[rf.lastApplied+1-rf.lastIncludedIndex():rf.commitIndex+1-rf.lastIncludedIndex()])
			rf.lastApplied = rf.commitIndex
			DPrintf(dLog2, "S%v applier, commitIndex: %v, lastApplied: %v, lastLogIndex:%v", rf.me, rf.commitIndex, rf.lastApplied, rf.getLastLogL().Index)
			rf.mu.Unlock()
			for _, entry := range entries {
				rf.applyCh <- ApplyMsg{
					CommandValid: true,
					Command:      entry.Command,
					CommandIndex: entry.Index,
					CommandTerm:  entry.Term,
				}
			}
		}
	}
}

func (rf *Raft) snapshoter(snapshot *[]byte, term int, index int) {
	rf.applyCh <- ApplyMsg{
		SnapshotValid: true,
		Snapshot:      *snapshot,
		SnapshotTerm:  term,
		SnapshotIndex: index,
	}
}

// the service or tester wants to create a Raft server. the ports
// of all the Raft servers (including this one) are in peers[]. this
// server's port is peers[me]. all the servers' peers[] arrays
// have the same order. persister is a place for this server to
// save its persistent state, and also initially holds the most
// recent saved state, if any. applyCh is a channel on which the
// tester or service expects Raft to send ApplyMsg messages.
// Make() must return quickly, so it should start goroutines
// for any long-running work.
func Make(peers []*labrpc.ClientEnd, me int,
	persister *Persister, applyCh chan ApplyMsg) *Raft {
	rf := &Raft{
		mu:             sync.Mutex{},
		peers:          peers,
		persister:      persister,
		me:             me,
		dead:           0,
		currentTerm:    0,
		votedFor:       NULL,
		commitIndex:    0,
		lastApplied:    0,
		state:          Follower,
		applyCh:        applyCh,
		heartbeatTimer: time.NewTimer(getHeartbeatDuration()),
		electionTimer:  time.NewTimer(getElectionDuration()),
		applyWaker:     make(chan int, 1000),
		log:            make([]Entry, 1),
		nextIndex:      make([]int, len(peers)),
		matchIndex:     make([]int, len(peers)),
	}
	// initialize from state persisted before a crash

	rf.readPersist(persister.ReadRaftState())
	// start ticker goroutine to start elections

	go rf.ticker()
	go rf.applier()
	return rf
}
