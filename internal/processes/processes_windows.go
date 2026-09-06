package processes

import (
	"fmt"
	"golang.org/x/sys/windows"
	"path/filepath"
	"sort"
	"speedlimitfree/internal/contracts"
	"strconv"
	"unsafe"
)

func Get(pid uint32) (contracts.Process, error) {
	p := contracts.Process{PID: pid}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return p, err
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, 32768)
	size := uint32(len(buf))
	if err = windows.QueryFullProcessImageName(h, 0, &buf[0], &size); err != nil {
		return p, err
	}
	p.Path = windows.UTF16ToString(buf[:size])
	p.Name = filepath.Base(p.Path)
	var created, exited, kernel, user windows.Filetime
	if err = windows.GetProcessTimes(h, &created, &exited, &kernel, &user); err != nil {
		return p, err
	}
	p.Started = strconv.FormatUint(uint64(created.HighDateTime)<<32|uint64(created.LowDateTime), 10)
	return p, nil
}

func List() ([]contracts.Process, error) {
	h, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(h)
	e := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	result := make([]contracts.Process, 0, 128)
	for err = windows.Process32First(h, &e); err == nil; err = windows.Process32Next(h, &e) {
		if e.ProcessID == 0 {
			continue
		}
		p, getErr := Get(e.ProcessID)
		if getErr != nil {
			p = contracts.Process{PID: e.ProcessID, Name: windows.UTF16ToString(e.ExeFile[:])}
		}
		result = append(result, p)
	}
	if err != windows.ERROR_NO_MORE_FILES {
		return nil, fmt.Errorf("enumerate processes: %w", err)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Name == result[j].Name {
			return result[i].PID < result[j].PID
		}
		return result[i].Name < result[j].Name
	})
	return result, nil
}
