# Lab1 MapReduce Design 

https://pdos.csail.mit.edu/6.824/labs/lab-mr.html

### Worker

空闲时，定时（1s）通过 rpc 向 coordinator 请求 task:
- 请求成功返回 task (task 类型，Id, 文件列表)，根据 task 类型分别进行 map/reduce，调用 plugin 定义的 map/reduce
- 若 task 为空，继续定时请求（可能是 map task 分配完但还未执行完）
- 若请求失败，认为 job 完成，退出（break 出循环）

任务成功:
map 或 reduce 任务成功后，向 coordinator 发送任务成功 rpc。带任务类型与任务 ID

### coordinator 实现1

coordinator 结构定义
```
type Coordinator struct {
	RunningMapTaskMap    map[int]*MapTask
	RunningReduceTaskMap map[int]*ReduceTask
	ActiveMapTasks       []*MapTask
	ActiveReduceTasks    []*ReduceTask
	ReduceN              int
}

type MapTask struct {
	TaskId   int
	FileName string
	Status   int32
}

type ReduceTask struct {
	TaskId   int
	FileName []string
	Status   int32 //0:NOTRUN 1:RUNNING 2:FINISHED
}
```



处理 worker 的 task 请求情况（需加锁操作）
- 如果 ActiveMapTasks 不为空，返回 map 任务：修改任务状态，RunningMapTaskMap 任务增加，ActiveMapTasks 任务删除。
- 若 ActiveMapTasks 为空，查看 RunningMapTaskMap , 如不为空说明 map 任务还未执行完，不做操作。
- 若 ActiveMapTasks 与 RunningMapTaskMap 为空，则返回 ActiveReduceTasks 中的 reduce 任务： 修改任务状态，RunningReduceTaskMap 任务增加，ActiveReduceTasks 任务删除。
- 若 ActiveMapTasks 与 RunningMapTaskMap ActiveReduceTasks 都为空，不操作

如何处理超时 （CAS）
- 失败/超时 重试实现：任务发送后，开启一个定时任务，10s 后根据状态检查任务是否完成(CAS 修改状态为0)。若修改成功，则重新加入 ActiveTasks，从 RunningTaskMap 中删除


处理 worker 的任务成功请求：
- 根据 taskId 获取 task，并 CAS 修改状态为 2（FINISHED）。若修改成功，则从 RunningTaskMap 中删除

如何判断任务完成
- 判断 ActiveReduceTasks 且 RunningReduceTaskMap 为空

### coordinator 实现2

coordinator 结构定义
```
type Coordinator struct {
	MapTaskMap        map[int]*MapTask
	ReduceTaskMap     map[int]*ReduceTask
	ActiveMapTasks    []*MapTask
	ActiveReduceTasks []*ReduceTask
	ReduceN           int
	mutex             sync.Mutex
}

type MapTask struct {
	TaskId   int
	FileName string
	Status   int32
}

type ReduceTask struct {
	TaskId   int
	FileName []string
	Status   int32 //0:NOTRUN 1:RUNNING 2:FINISHED
}
```



处理 worker 的 task 请求情况（需加锁操作）
- ActiveMapTasks 含0状态，返回相应 map 任务：修改任务状态为1
- ActiveMapTasks 不含0状态，则检测是否全为2。是的话查看 ActiveReduceTasks 含0状态 , 是的话返回 相应 reduce 任务：修改任务状态为1
- 若 ActiveMapTasks 不含0状态 ，但不全为2 （有1）则返回空；同理 ActiveReduceTasks 含1 ，返回空

如何处理超时 （CAS）
- 失败/超时 重试实现：任务发送后，开启一个定时任务，10s 后根据状态检查任务是否完成(CAS 修改状态为0)


处理 worker 的任务成功请求：
- 根据 taskId 获取 task，并 CAS 修改状态为 2

如何判断任务完成
- 判断 ActiveReduceTasks 中是否全2

### How to Run & Test


```
$ cd ~/6.824/src/main
go build -race -buildmode=plugin ../mrapps/wc.go
```

```
rm mr-out*
rm mr-*
go run -race mrcoordinator.go pg-*.txt
```

in other terminal, run several worker
```
$ go run -race mrworker.go wc.so
```

Test:
```
$ cd ~/6.824/src/main
$ bash test-mr.sh
```

# RAFT 2A

1. 如何触发选举

- for 循环来循环执行
- 生成随机的 election timeout
- sleep election timeout，然后执行检查
- 若当前节点不为 leader 且节点距离上次心跳时间已经超过 election timeout，则开始选举（开新的线程）

关键点
- 这种方式实际上当 follower 收到心跳，并不是所谓的重置 election timeout，而是重新计算了心跳开始时间。election timeout 的重置均在 election timeout 结束时触发（包括 leader 的）
- 当选举超时，用这种方式也能让 candidate 重新进行选举

```
func (rf *Raft) ticker() {
	for rf.killed() == false {

		// Your code here to check if a leader election should
		// be started and to randomize sleeping time using
		// time.Sleep().

		// check after electionTimeout
		rand.Seed(time.Now().UnixNano())
		electionTimeout := time.Duration(rand.Intn(interval)+interval) * time.Millisecond
		time.Sleep(electionTimeout)
		rf.mu.Lock()
		if (rf.state != LEADER) && (time.Now().Sub(rf.lastHeartbeatTime) > electionTimeout) {
			go rf.AttemptEletion()
		}
		rf.mu.Unlock()
	}
}
```

1.1 如何触发选举2
> 这是第二种方法

使用 timer 定时器执行
- 定时器可以在一定时间后执行（election timeout）
- 当收到心跳，定时器也可以停止重置 Stop()，然后重新定时
- 发出选票后，需要重置 election timeout ,即超时失败重试

和 sleep 区别：
- 用 sleep 的话自己就会不断循环了,但收到心跳时无法打断 sleep 重新开始
- 用 timer 更准确，每次心跳都可以重置，也必须去重置定时器（因为它本身不循环），或许会多耗一些内存

```
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
		rf.electionTimeout = time.AfterFunc(time.Duration(rand.Intn(interval)+interval)*time.Millisecond, func() {
			rf.mu.Lock()
			if rf.state != LEADER {
				go rf.AttemptEletion()
			}
			rf.mu.Unlock()
		})
	}
}
```

2. 如何发起投票

- 转变自己为 candidate，加任期，投自己一票（加锁）
- 多线程发起投票
  - 若对方任期更大，转为 follower (重置 election timeout  与 votefor)
  - 若对方同意，票加一，并计算是否取得多数票了（在每个 goroutine 里判断）
  - 若第一次得到多数票，则进行判断（自己是否为 candidate，任期是否还一致，判断通过当选 leader

keypoint
- 在个 goroutine 里判断票数，可以减少等待。否则需要在外层不断循环检查 
- 任期与 candidate 的判断，是为了防止在同一任期出现双主：考虑3节点。0节点触发选举得到2票—> 1节点任期已增加触发选举，同样能得到2票 -> 0节点任期增加 -> 0节点处理返回结果当选 leader -> 1节点当选 leader。导致同一任期双主。其主要原因在于 0 节点在处理返回结果之前，由于是 rpc 请求没有对任期加锁，导致任期增加

3.处理投票

- 先判断任期，若任期小，则更新任期与 votefor; 若任期大，则直接返回 false；若任期相同，什么也不做
- 已经投过了，且投的不是该节点。则返回 false
- 没投过则成功。投完重置 election timeout 与 votefor

4. 发起心跳

- leader 才能发起心跳
- 心跳结果如果对方任期大，自己降级为 follower (重置 election timeout  与 votefor)

5. 处理心跳

- 若自己不为 follower : 比较任期大小，若 <= 心跳任期，则自己切换为 follower；否则，返回失败
- 若自己为 follower: 比较任期大小，若 <= 心跳任期，则心跳成功；否则，返回失败
- 心跳成功时，重置 election timeout