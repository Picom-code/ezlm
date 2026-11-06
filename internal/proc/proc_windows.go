package proc

import (
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// A job object with KILL_ON_JOB_CLOSE: closing the handle (explicitly, or
// because EZLM exited) kills every process in it, including grandchildren.
//
// proc is the root process's handle. It is the fallback when there is no
// usable job (CreateJobObject failed, the process already belongs to a job
// that forbids nesting, or it exited before it could be assigned): without
// it a timeout waited forever for a child the job never owned, and the
// leftovers of a finished command were never killed.
type tree struct {
	job  windows.Handle
	proc windows.Handle
}

// prepare keeps helper consoles (cmd.exe, npm, powershell) from flashing up.
func prepare(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_NO_WINDOW
}

func track(cmd *exec.Cmd) *tree {
	t := &tree{}
	if h, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid)); err == nil {
		t.proc = h
	}
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return t
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	// A job without KILL_ON_JOB_CLOSE would let close() return having
	// killed nothing, so only keep the job if the limit was set.
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil || t.proc == 0 {
		windows.CloseHandle(job)
		return t
	}
	if err := windows.AssignProcessToJobObject(job, t.proc); err != nil {
		windows.CloseHandle(job)
		return t
	}
	t.job = job
	return t
}

func (t *tree) kill() {
	if t.job != 0 {
		windows.TerminateJobObject(t.job, 1)
		return
	}
	if t.proc != 0 {
		windows.TerminateProcess(t.proc, 1)
	}
}

// close kills anything the command left behind (a finished check must not
// leave a client talking to the model).
func (t *tree) close() {
	if t.job != 0 {
		windows.CloseHandle(t.job) // KILL_ON_JOB_CLOSE does the killing
		t.job = 0
	} else if t.proc != 0 {
		windows.TerminateProcess(t.proc, 1)
	}
	if t.proc != 0 {
		windows.CloseHandle(t.proc)
		t.proc = 0
	}
}

// release lets the remaining processes live on after the job handle closes.
func (t *tree) release() {
	if t.job != 0 {
		info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
		windows.SetInformationJobObject(t.job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)))
		windows.CloseHandle(t.job)
		t.job = 0
	}
	if t.proc != 0 {
		windows.CloseHandle(t.proc)
		t.proc = 0
	}
}

// pids lists the processes still in the job.
func (t *tree) pids() []int {
	if t.job == 0 {
		return nil
	}
	buf := make([]byte, jobPidsSize)
	for {
		var ret uint32
		err := windows.QueryInformationJobObject(t.job, windows.JobObjectBasicProcessIdList, uintptr(unsafe.Pointer(&buf[0])), uint32(len(buf)), &ret)
		if err == nil {
			return parseJobPids(buf, int(unsafe.Sizeof(uintptr(0))))
		}
		// Too small: Windows reports the size it needs, so ask again.
		if err != windows.ERROR_MORE_DATA || int(ret) <= len(buf) {
			return nil
		}
		buf = make([]byte, ret)
	}
}
