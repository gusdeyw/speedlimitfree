package main

import (
	"context"
	"flag"
	"fmt"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"speedlimitfree/internal/contracts"
	"speedlimitfree/internal/engine"
	"speedlimitfree/internal/ipc"
	"time"
)

type service struct {
	owner, data, driver string
	monitor             bool
}

func (s *service) run(ctx context.Context, ready func()) error {
	eng, err := engine.New(filepath.Join(s.data, "rules.json"))
	if err != nil {
		return fmt.Errorf("load saved rules: %w", err)
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := eng.Run(ctx, s.driver, s.monitor); err != nil {
			log.Printf("engine stopped: %v", err)
		}
	}()
	err = ipc.ServeReady(ctx, s.owner, eng, ready)
	cancel()
	<-done
	if err != nil {
		return fmt.Errorf("service control pipe: %w", err)
	}
	return err
}
func (s *service) Execute(_ []string, r <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	current := svc.Status{State: svc.StartPending, CheckPoint: 1, WaitHint: 10000}
	status <- current
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	ready := make(chan struct{})
	go func() { done <- s.run(ctx, func() { close(ready) }) }()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ready:
			ready = nil
			current = svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
			status <- current
			log.Print("service control pipe ready")
		case <-ticker.C:
			if current.State == svc.StartPending {
				current.CheckPoint++
				status <- current
			}
		case err := <-done:
			if err != nil {
				log.Print(err)
				return true, 1
			}
			return false, 0
		case req := <-r:
			switch req.Cmd {
			case svc.Interrogate:
				status <- current
			case svc.Stop, svc.Shutdown:
				current = svc.Status{State: svc.StopPending, CheckPoint: 1, WaitHint: 10000}
				status <- current
				cancel()
				for {
					select {
					case err := <-done:
						if err != nil {
							log.Print(err)
							return true, 1
						}
						log.Print("service stopped")
						return false, 0
					case <-ticker.C:
						current.CheckPoint++
						status <- current
					}
				}
			}
		}
	}
}
func main() {
	exe, _ := os.Executable()
	s := &service{}
	serviceMode := flag.Bool("service", false, "run under Windows Service Control Manager")
	flag.StringVar(&s.owner, "owner", "", "SID allowed to control this service")
	flag.StringVar(&s.data, "data", filepath.Join(os.Getenv("ProgramData"), "SpeedLimitFree"), "settings directory")
	flag.StringVar(&s.driver, "driver", filepath.Dir(exe), "directory containing WinDivert.dll and WinDivert64.sys")
	flag.BoolVar(&s.monitor, "monitor-only", false, "list real processes without packet interception")
	flag.Parse()
	if s.owner == "" {
		token := windows.GetCurrentProcessToken()
		u, err := token.GetTokenUser()
		if err != nil {
			log.Fatal(err)
		}
		s.owner = u.User.Sid.String()
	}
	if *serviceMode {
		if err := os.MkdirAll(s.data, 0700); err != nil {
			log.Fatal(err)
		}
		f, err := os.OpenFile(filepath.Join(s.data, "service.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()
		log.SetOutput(f)
		log.Print("service starting")
		if err = svc.Run(contracts.ServiceName, s); err != nil {
			log.Fatal(err)
		}
		return
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	fmt.Printf("SpeedLimitFree service console; owner %s\n", s.owner)
	if err := s.run(ctx, nil); err != nil {
		log.Fatal(err)
	}
}
