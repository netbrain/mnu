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
    p := tea.NewProgram(runner.InitialModel())
    m, err := p.Run()
    if err != nil {
        fmt.Printf("mnu-run: %v\n", err)
        os.Exit(1)
    }
    if rm, ok := m.(runner.Model); ok {
        if rm.SelectedPath() != "" {
            devNull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
            if err != nil { fmt.Printf("failed to open %s: %v\n", os.DevNull, err); os.Exit(1) }
            defer devNull.Close()

            launchPath := rm.SelectedPath()
            cmd := exec.Command(launchPath)
            cmd.Stdin = devNull
            cmd.Stdout = devNull
            cmd.Stderr = devNull
            cmd.SysProcAttr = &syscall.SysProcAttr{ Setsid: true }
            if err := cmd.Start(); err != nil {
                fmt.Printf("failed to start %s: %v\n", launchPath, err)
                os.Exit(1)
            }
            return
        }
    }
}
