package main

import (
    "fmt"
    "os"
    "os/exec"
    "syscall"

    tea "github.com/charmbracelet/bubbletea"
    runner "github.com/netbrain/mnu/internal/runner"
)

func main() {
    p := tea.NewProgram(runner.InitialDesktopModel())
    m, err := p.Run()
    if err != nil {
        fmt.Printf("mnu-drun: %v\n", err)
        os.Exit(1)
    }
    if dm, ok := m.(runner.DesktopModel); ok {
        execStr := dm.SelectedExec()
        if execStr != "" {
            devNull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
            if err != nil { fmt.Printf("failed to open %s: %v\n", os.DevNull, err); os.Exit(1) }
            defer devNull.Close()
            cmd := exec.Command("/bin/sh", "-c", execStr)
            cmd.Stdin = devNull
            cmd.Stdout = devNull
            cmd.Stderr = devNull
            cmd.SysProcAttr = &syscall.SysProcAttr{ Setsid: true }
            if err := cmd.Start(); err != nil {
                fmt.Printf("failed to start desktop app: %v\n", err)
                os.Exit(1)
            }
            return
        }
    }
}
