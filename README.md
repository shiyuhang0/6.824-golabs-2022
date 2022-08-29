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