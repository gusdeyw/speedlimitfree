package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"speedlimitfree/internal/contracts"
	"speedlimitfree/internal/engine"
	"speedlimitfree/internal/ipc"
	"speedlimitfree/internal/processes"
	"speedlimitfree/internal/servicectl"
	"time"
)

func main() {
	pid := flag.Uint("pid", 0, "process ID to limit")
	download := flag.Int64("download", 0, "download bytes/s; 0 means unlimited")
	upload := flag.Int64("upload", 0, "upload bytes/s; 0 means unlimited")
	driver := flag.String("driver", ".", "WinDivert directory")
	list := flag.Bool("list", false, "list processes")
	status := flag.Bool("status", false, "query the running service")
	serviceState := flag.Bool("service-state", false, "read Windows service state and logs without elevation")
	serviceAction := flag.String("service-action", "", "manage the service: install, start, stop, restart (Windows administrator prompt)")
	flag.Parse()
	if *serviceState {
		s, err := servicectl.Status()
		if err != nil {
			log.Fatal(err)
		}
		json.NewEncoder(os.Stdout).Encode(s)
		return
	}
	if *serviceAction != "" {
		exe, err := os.Executable()
		if err != nil {
			log.Fatal(err)
		}
		r, err := servicectl.Manage(context.Background(), *serviceAction, filepath.Dir(exe))
		if err != nil {
			log.Fatal(err)
		}
		json.NewEncoder(os.Stdout).Encode(r)
		return
	}
	if *status {
		s, err := ipc.Call(context.Background(), contracts.Request{Method: "snapshot"})
		if err != nil {
			log.Fatal(err)
		}
		json.NewEncoder(os.Stdout).Encode(s)
		return
	}
	if *list {
		ps, err := processes.List()
		if err != nil {
			log.Fatal(err)
		}
		for _, p := range ps {
			fmt.Printf("%6d  %-32s %s\n", p.PID, p.Name, p.Path)
		}
		return
	}
	if *pid == 0 || *pid > 1<<32-1 {
		log.Fatal("provide a valid --pid, --list, or --status")
	}
	p, err := processes.Get(uint32(*pid))
	if err != nil {
		log.Fatal(err)
	}
	dir, err := os.MkdirTemp("", "speedlimitfree-debug-")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dir)
	e, err := engine.New(filepath.Join(dir, "rules.json"))
	if err != nil {
		log.Fatal(err)
	}
	r := contracts.Rule{Name: p.Name, Path: p.Path, Scope: "process", PID: p.PID, Started: p.Started, Enabled: true}
	if *download != 0 {
		r.Download = download
	}
	if *upload != 0 {
		r.Upload = upload
	}
	if err = e.Command(contracts.Request{Method: "save", Rule: &r}); err != nil {
		log.Fatal(err)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- e.Run(ctx, *driver, false) }()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			<-done
			return
		case err := <-done:
			if err != nil {
				log.Print(err)
			}
			return
		case <-ticker.C:
			s := e.Snapshot()
			fmt.Printf("%s | down %.0f B/s | up %.0f B/s | queue %d B | drops %d | %s\n", s.Engine, s.Download, s.Upload, s.QueueBytes, s.Dropped, s.Message)
		}
	}
}
