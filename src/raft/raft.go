package raft

//
// this is an outline of the API that raft must expose to
// the service (or tester). see comments below for
// each of these functions for more details.
//
// rf = Make(...)
//   create a new Raft server.
// rf.Start(command interface{}) (index, term, isleader)
//   start agreement on a new log entry
// rf.GetState() (term, isLeader)
//   ask a Raft for its current term, and whether it thinks it is leader
// ApplyMsg
//   each time a new entry is committed to the log, each Raft peer
//   should send an ApplyMsg to the service (or tester)
//   in the same server.
//

import (
	"log"
	"math/rand"
	"time"

	//	"bytes"
	"sync"
	"sync/atomic"
	//	"6.824/labgob"
	"6.824/labrpc"
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
type ApplyMsg struct {
	CommandValid bool
	Command      interface{}
	CommandIndex int

	// For 2D:
	SnapshotValid bool
	Snapshot      []byte
	SnapshotTerm  int
	SnapshotIndex int
}

// A Go object implementing a single Raft peer.
type Raft struct {
	mu        sync.Mutex          // Lock to protect shared access to this peer's state
	peers     []*labrpc.ClientEnd // RPC end points of all peers
	persister *Persister          // Object to hold this peer's persisted state
	me        int                 // this peer's index into peers[]
	dead      int32               // set by Kill()

	currentTerm int
	votedFor    int
	log         []LogEntry

	state           State
	electionTimeout *time.Timer

	commitIndex int
	lastApplied int

	// init when turn to leader
	nextIndex  []int
	matchIndex []int

	termToIndex map[int]int
}

const interval int = 200

type LogEntry struct {
	Term    int
	Command interface{}
}

// return currentTerm and whether this server
// believes it is the leader.
func (rf *Raft) GetState() (int, bool) {

	var term int
	var isleader bool
	// Your code here (2A).
	rf.mu.Lock()
	term = rf.currentTerm
	isleader = rf.state == LEADER
	rf.mu.Unlock()
	return term, isleader
}

type State int

const (
	LEADER    State = 1
	FOLLOWER  State = 2
	CANDIDATE State = 3
)

func (rf *Raft) conevrtToCandidate() {
	DPrintf("%d turn to Candidate", rf.me)

	rf.state = CANDIDATE
	rf.currentTerm++
	rf.votedFor = rf.me
}

func (rf *Raft) conevrtToFollower(term int) {
	DPrintf("%d turn to Follower", rf.me)
	rf.state = FOLLOWER
	rf.currentTerm = term
	rf.votedFor = -1
	// regard as a heartbeat
	rf.ticker()
}

func (rf *Raft) conevrtToLeader() {
	rf.state = LEADER

	rf.nextIndex = make([]int, 0)
	rf.matchIndex = make([]int, 0)
	for range rf.peers {
		rf.nextIndex = append(rf.nextIndex, len(rf.log))
		rf.matchIndex = append(rf.matchIndex, 0)
	}

	// time.Sleep(50 * time.Millisecond)
	go rf.HeartBeat()
}

func (rf *Raft) AttemptEletion() {
	rf.mu.Lock()
	rf.conevrtToCandidate()
	votes := 1
	done := false
	args := &RequestVoteArgs{
		Term:         rf.currentTerm,
		CandidateId:  rf.me,
		LastLogIndex: len(rf.log) - 1,
		LastLogTerm:  rf.log[len(rf.log)-1].Term,
	}

	rf.mu.Unlock()

	log.Printf("[%d] start sendRequestVote, term: %d", rf.me, rf.currentTerm)
	for index := range rf.peers {
		if index == rf.me {
			continue
		}
		go func(index int) {
			reply := &RequestVoteReply{}
			ok := rf.sendRequestVote(index, args, reply)
			if !ok {
				DPrintf("[%d] sendRequestVote fail to %d ", rf.me, index)
				return
			}

			rf.mu.Lock()
			defer rf.mu.Unlock()

			// become follower when other term is bigger than current term
			if reply.Term > rf.currentTerm {
				log.Printf("[%d] turn to follower in request vote for get bigger term %d from %d ", rf.me, reply.Term, index)
				rf.conevrtToFollower(reply.Term)
				return
			}

			// return when we did not get vote
			if !reply.VoteGranted {
				log.Printf("[%d] did not get vote from %d ", rf.me, index)
				return
			}

			// handle the vote
			votes++
			log.Printf("[%d] got vote from %d ", rf.me, index)
			if done || votes <= len(rf.peers)/2 {
				return
			}
			// double check because we did not lock when we execute RPC
			if rf.currentTerm == args.Term && rf.state == CANDIDATE {
				done = true
				log.Printf("[%d] turn to leader (currentTerm: %d,state %v) ", rf.me, rf.currentTerm, rf.state)
				rf.conevrtToLeader()
			}
		}(index)
	}

	// retry when election failed
	rf.ticker()

}

// save Raft's persistent state to stable storage,
// where it can later be retrieved after a crash and restart.
// see paper's Figure 2 for a description of what should be persistent.
func (rf *Raft) persist() {
	// Your code here (2C).
	// Example:
	// w := new(bytes.Buffer)
	// e := labgob.NewEncoder(w)
	// e.Encode(rf.xxx)
	// e.Encode(rf.yyy)
	// data := w.Bytes()
	// rf.persister.SaveRaftState(data)
}

// restore previously persisted state.
func (rf *Raft) readPersist(data []byte) {
	if data == nil || len(data) < 1 { // bootstrap without any state?
		return
	}
	// Your code here (2C).
	// Example:
	// r := bytes.NewBuffer(data)
	// d := labgob.NewDecoder(r)
	// var xxx
	// var yyy
	// if d.Decode(&xxx) != nil ||
	//    d.Decode(&yyy) != nil {
	//   error...
	// } else {
	//   rf.xxx = xxx
	//   rf.yyy = yyy
	// }
}

// A service wants to switch to snapshot.  Only do so if Raft hasn't
// have more recent info since it communicate the snapshot on applyCh.
func (rf *Raft) CondInstallSnapshot(lastIncludedTerm int, lastIncludedIndex int, snapshot []byte) bool {

	// Your code here (2D).

	return true
}

// the service says it has created a snapshot that has
// all info up to and including index. this means the
// service no longer needs the log through (and including)
// that index. Raft should now trim its log as much as possible.
func (rf *Raft) Snapshot(index int, snapshot []byte) {
	// Your code here (2D).

}

type AppendEntriesArgs struct {
	Term         int
	LeaderId     int
	PrevLogIndex int
	PrevLogTerm  int
	Entries      []LogEntry
	LeaderCommit int
	TermToIndex  map[int]int
}

type AppendEntriesReply struct {
	Term    int
	Success bool
}

func (rf *Raft) AppendEntries(args *AppendEntriesArgs, reply *AppendEntriesReply) {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	if rf.state != FOLLOWER {
		if rf.currentTerm <= args.Term {
			log.Printf("[%d] turn to Followers because get bigger term when handle heartbeat send by [%d] ", rf.me, args.LeaderId)
			rf.conevrtToFollower(args.Term)
			reply.Term = rf.currentTerm
			reply.Success = true
			return
		} else {
			reply.Term = rf.currentTerm
			reply.Success = false
			return
		}
	}
	if rf.currentTerm <= args.Term {
		rf.currentTerm = args.Term
		rf.ticker()
		reply.Term = rf.currentTerm

		index := args.PrevLogIndex
		// return false when don't have entry at PrevLogIndex
		if len(rf.log) <= index {
			reply.Success = false
			return
		}
		// return false when term do not match at PrevLogIndex
		if rf.log[index].Term != args.PrevLogTerm {
			//rf.log = rf.log[:index]
			reply.Success = false
			return
		}

		// append other logs
		rf.log = rf.log[:index+1]
		for _, value := range args.Entries {
			DPrintf("[%d] append logs:%v", rf.me, value)
			rf.log = append(rf.log, value)
		}

		// copy termtoindex map
		rf.termToIndex = args.TermToIndex

		// alter commitIndex
		if args.LeaderCommit > rf.commitIndex {
			if args.LeaderCommit < len(rf.log)-1 {
				rf.commitIndex = args.LeaderCommit
			} else {
				rf.commitIndex = len(rf.log) - 1
			}
		}
		reply.Success = true
		return
	} else {
		reply.Term = rf.currentTerm
		reply.Success = false
		return
	}
}

func (rf *Raft) sendAppendEntries(server int, args *AppendEntriesArgs, reply *AppendEntriesReply) bool {
	ok := rf.peers[server].Call("Raft.AppendEntries", args, reply)
	return ok
}

// example RequestVote RPC arguments structure.
// field names must start with capital letters!
type RequestVoteArgs struct {
	// Your data here (2A, 2B).
	Term         int
	CandidateId  int
	LastLogIndex int
	LastLogTerm  int
}

// example RequestVote RPC reply structure.
// field names must start with capital letters!
type RequestVoteReply struct {
	// Your data here (2A).
	Term        int
	VoteGranted bool
}

// example RequestVote RPC handler.
func (rf *Raft) RequestVote(args *RequestVoteArgs, reply *RequestVoteReply) {
	rf.mu.Lock()
	defer rf.mu.Unlock()

	// Reply false if term < currentTerm
	if rf.currentTerm > args.Term {
		//log.Printf("0:[%d] did not vote to %d",rf.me,args.CandidateId)
		reply.Term = rf.currentTerm
		reply.VoteGranted = false
		return
	}

	if rf.currentTerm < args.Term {
		rf.currentTerm = args.Term
		rf.votedFor = -1
	}

	// do not grant false if have voted
	if rf.votedFor != -1 && rf.votedFor != args.CandidateId {
		//log.Printf("1:[%d] did not vote to %d", rf.me, args.CandidateId)
		reply.VoteGranted = false
		reply.Term = rf.currentTerm
		return
	}

	// only grant when leader's log is newer
	if args.LastLogTerm > rf.log[len(rf.log)-1].Term || (args.LastLogTerm == rf.log[len(rf.log)-1].Term && args.LastLogIndex >= len(rf.log)-1) {
		reply.VoteGranted = true
		reply.Term = rf.currentTerm
		rf.votedFor = args.CandidateId
		//reset election timeout after vote successfully
		rf.ticker()
	} else {
		reply.VoteGranted = false
		return
	}

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
// look at the comments in ../labrpc/labrpc.go for more details.
//
// if you're having trouble getting RPC to work, check that you've
// capitalized all field names in structs passed over RPC, and
// that the caller passes the address of the reply struct with &, not
// the struct itself.
func (rf *Raft) sendRequestVote(server int, args *RequestVoteArgs, reply *RequestVoteReply) bool {
	ok := rf.peers[server].Call("Raft.RequestVote", args, reply)
	return ok
}

// the service using Raft (e.g. a k/v server) wants to start
// agreement on the next command to be appended to Raft's log. if this
// server isn't the leader, returns false. otherwise start the
// agreement and return immediately. there is no guarantee that this
// command will ever be committed to the Raft log, since the leader
// may fail or lose an election. even if the Raft instance has been killed,
// this function should return gracefully.
//
// the first return value is the index that the command will appear at
// if it's ever committed. the second return value is the current
// term. the third return value is true if this server believes it is
// the leader.
func (rf *Raft) Start(command interface{}) (int, int, bool) {
	index := -1
	term := -1
	isLeader := true

	// Your code here (2B).
	rf.mu.Lock()
	term = rf.currentTerm
	index = len(rf.log) - 1
	isLeader = rf.state == LEADER
	if isLeader {
		if rf.currentTerm != rf.log[len(rf.log)-1].Term {
			rf.termToIndex[rf.currentTerm] = len(rf.log)
		}
		rf.log = append(rf.log, LogEntry{
			Term:    rf.currentTerm,
			Command: command,
		})
		index++
	}
	rf.mu.Unlock()

	return index, term, isLeader
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
	// Your code here, if desired.
}

func (rf *Raft) killed() bool {
	z := atomic.LoadInt32(&rf.dead)
	return z == 1
}

// The ticker go routine starts a new election if this peer hasn't received
// heartsbeats recently.
func (rf *Raft) ticker() {
	if rf.killed() == false {

		// Your code here to check if a leader election should
		// be started and to randomize sleeping time using
		// time.Sleep().

		if rf.electionTimeout != nil {
			rf.electionTimeout.Stop()
		}
		rand.Seed(time.Now().UnixNano())
		rf.electionTimeout = time.AfterFunc(time.Duration(rand.Intn(2*interval)+interval)*time.Millisecond, func() {
			rf.mu.Lock()
			if rf.state != LEADER {
				go rf.AttemptEletion()
			}
			rf.mu.Unlock()
		})
	}
}

func (rf *Raft) HeartBeat() {

	for rf.killed() == false {
		rf.mu.Lock()
		state := rf.state
		count := 1
		done := false
		rf.mu.Unlock()

		if state != LEADER {
			break
		}

		// send heartbeat to all peers
		if state == LEADER {
			for index := range rf.peers {
				if index == rf.me {
					continue
				}
				go func(index int) {
					rf.mu.Lock()
					args := &AppendEntriesArgs{
						Term:         rf.currentTerm,
						LeaderId:     rf.me,
						PrevLogIndex: rf.nextIndex[index] - 1,
						PrevLogTerm:  rf.log[rf.nextIndex[index]-1].Term,
						Entries:      rf.log[rf.nextIndex[index]:],
						LeaderCommit: rf.commitIndex,
						TermToIndex:  rf.termToIndex,
					}
					rf.mu.Unlock()
					reply := &AppendEntriesReply{}
					ok := rf.sendAppendEntries(index, args, reply)
					if !ok {
						DPrintf("[%d] sed HeartBeat to %d fail", rf.me, index)
						return
					}
					DPrintf("[%d] get HeartBeat reply from index %d,reply:%v", rf.me, index, reply)
					rf.mu.Lock()
					defer rf.mu.Unlock()
					if !reply.Success {
						// if reply.Term <= rf.currentTerm, we will regard as append log fail
						if reply.Term > rf.currentTerm {
							log.Printf("[%d] turn to Followers because get bigger term in heartbeat reply from %d", rf.me, index)
							rf.conevrtToFollower(reply.Term)
						} else {
							// use the last term's latest index
							rf.nextIndex[index] = 1
							for i := args.PrevLogTerm - 1; i > 0; i-- {
								if rf.termToIndex[i] != 0 {
									rf.nextIndex[index] = rf.termToIndex[i]
									break
								}
							}

						}
					} else {
						rf.nextIndex[index] = len(rf.log)
						rf.matchIndex[index] = len(rf.log) - 1
						count++
						if done || count <= len(rf.peers)/2 {
							return
						}
						done = true
						rf.commitIndex = len(rf.log) - 1
						DPrintf("[%d]commit index:%d", rf.me, rf.commitIndex)
					}
				}(index)
			}
		}
		time.Sleep(time.Duration(interval) * time.Millisecond)

	}
}

func (rf *Raft) apply(applyCh chan ApplyMsg) {
	for rf.killed() == false {
		time.Sleep(10 * time.Millisecond)
		if rf.commitIndex > rf.lastApplied {
			rf.lastApplied++
			DPrintf("%d apply index:%d command:%v term:%d", rf.me, rf.lastApplied, rf.log[rf.lastApplied].Command, rf.log[rf.lastApplied].Term)
			applymsg := ApplyMsg{
				CommandValid: true,
				CommandIndex: rf.lastApplied,
				Command:      rf.log[rf.lastApplied].Command,
			}
			applyCh <- applymsg
		}
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
	rf := &Raft{}
	rf.peers = peers
	rf.persister = persister
	rf.me = me

	// Your initialization code here (2A, 2B, 2C).
	rf.currentTerm = 0
	rf.votedFor = -1
	rf.state = FOLLOWER
	rf.log = make([]LogEntry, 0)
	// the log index should begin with 1, so fill index 0 with invalid log
	rf.log = append(rf.log, LogEntry{
		Term: 0,
	})
	rf.termToIndex = make(map[int]int)
	rf.termToIndex[0] = 0

	rf.commitIndex = 0
	rf.lastApplied = 0

	// initialize from state persisted before a crash
	rf.readPersist(persister.ReadRaftState())

	// start ticker goroutine to start elections
	go rf.ticker()

	go rf.apply(applyCh)

	return rf
}
