// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package appstats

import (
	"context"
	"os"
	"strconv"
	"sync"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

var stringBuffer = sync.Pool{New: func() any { b := make([]byte, 0, StringFormatBufferSize); return &b }}

func formatLoad(value float64, suffix string) string {
	bufPtr := stringBuffer.Get().(*[]byte) //nolint:forcetypeassert,errcheck // stringBuffer only contains *[]byte.
	buf := strconv.AppendFloat((*bufPtr)[:0], value, 'f', 2, 64)
	buf = append(buf, suffix...)
	formatted := string(buf)
	*bufPtr = buf
	stringBuffer.Put(bufPtr)
	return formatted
}

func collectServiceCPU(proc *process.Process) (string, bool) {
	value, err := proc.CPUPercent()
	if err != nil {
		return "", false
	}
	return formatLoad(value, "%"), true
}

func collectSystemCPU(ctx context.Context) (string, bool) {
	// A zero interval uses the previous system reading without sleeping.
	values, err := cpu.PercentWithContext(ctx, 0, false)
	if err != nil || len(values) == 0 {
		return "", false
	}
	return formatLoad(values[0], "%"), true
}

func collectMemory(ctx context.Context, cachedProcess func() *process.Process) *MemoryLoadData {
	vmStat, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		return nil
	}

	// Get process if available
	proc := cachedProcess()
	if proc == nil {
		var processErr error
		proc, processErr = ProcessDetails(ctx)
		if processErr != nil {
			return nil
		}
	}

	memInfo, err := proc.MemoryInfoWithContext(ctx)
	if err != nil {
		return nil
	}
	const percentScale = 100
	return &MemoryLoadData{
		Service: formatLoad(float64(memInfo.RSS)/float64(vmStat.Total)*percentScale, "%"),
		System:  formatLoad(vmStat.UsedPercent, "%"),
		Total:   formatLoad(float64(vmStat.Total), ""),
	}
}

func collectNetwork() (received, sent uint64, err error) {
	netIO, err := net.IOCounters(true)
	if err != nil {
		return 0, 0, err
	}

	for i := range netIO {
		received += netIO[i].BytesRecv
		sent += netIO[i].BytesSent
	}
	return received, sent, nil
}

// ProcessDetails returns the process details for the current application.
// It creates a new process object using the current process ID and can be used
// to query detailed process information such as CPU usage, memory consumption, etc.
//
// The context can be used to cancel the operation if it takes too long.
//
// Returns a process.Process object for the current application process,
// or an error if the process cannot be found or accessed.
func ProcessDetails(ctx context.Context) (*process.Process, error) {
	pid := ProcessId()
	proc, err := process.NewProcessWithContext(ctx, pid)
	if err != nil {
		return nil, coreerrs.Wrapf(err, "get process details for pid %d", pid)
	}
	return proc, nil
}

// ProcessId returns the process ID of the current application.
// This is a wrapper around os.Getpid() that returns the PID as int32
// for compatibility with the gopsutil library.
//
// Returns the current process ID as int32.
func ProcessId() int32 {
	return int32(os.Getpid()) // #nosec G115 -- PIDs fit in int32 on all supported platforms
}
