// 文件与进程均由测试使用临时目录创建；所有等待有退出上限。
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		os.Exit(2)
	}
	switch os.Args[1] {
	case "inspect":
		cwd, err := os.Getwd()
		if err != nil {
			panic(err)
		}
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			panic(err)
		}
		if err := json.NewEncoder(os.Stdout).Encode(map[string]any{"args": os.Args[2:], "cwd": cwd, "credential": os.Getenv("OPENAI_API_KEY"), "stdin_bytes": len(data)}); err != nil {
			panic(err)
		}
	case "write":
		if err := os.WriteFile(os.Args[2], []byte("executed"), 0600); err != nil {
			panic(err)
		}
		fmt.Print("written")
	case "exit":
		fmt.Fprint(os.Stdout, "out")
		fmt.Fprint(os.Stderr, "err")
		code, err := strconv.Atoi(os.Args[2])
		if err != nil {
			panic(err)
		}
		os.Exit(code)
	case "stream":
		fmt.Fprint(os.Stdout, "first中文\n")
		until := time.Now().Add(10 * time.Second)
		for {
			if _, err := os.Stat(os.Args[2]); err == nil {
				break
			}
			if time.Now().After(until) {
				os.Exit(3)
			}
			time.Sleep(10 * time.Millisecond)
		}
		fmt.Fprint(os.Stderr, "diagnostic\n")
		fmt.Fprint(os.Stdout, "second\n")
	case "large":
		fmt.Fprint(os.Stdout, strings.Repeat("\x00\"中文", 100000))
		fmt.Fprint(os.Stderr, strings.Repeat("\xff", 200000))
	case "sleep":
		fmt.Fprint(os.Stdout, "started\n")
		time.Sleep(20 * time.Second)
	case "spawn", "spawn-exit":
		executable, err := os.Executable()
		if err != nil {
			panic(err)
		}
		child := exec.Command(executable, "heartbeat", os.Args[2])
		child.Stdout, child.Stderr = os.Stdout, os.Stderr
		if err := child.Start(); err != nil {
			panic(err)
		}
		if err := os.WriteFile(os.Args[2]+".pid", []byte(strconv.Itoa(child.Process.Pid)), 0600); err != nil {
			panic(err)
		}
		for until := time.Now().Add(5 * time.Second); ; {
			if _, err := os.Stat(os.Args[2]); err == nil {
				break
			}
			if time.Now().After(until) {
				os.Exit(3)
			}
			time.Sleep(10 * time.Millisecond)
		}
		fmt.Fprint(os.Stdout, "child-ready\n")
		if os.Args[1] == "spawn-exit" {
			return
		}
		time.Sleep(20 * time.Second)
	case "heartbeat":
		for until := time.Now().Add(20 * time.Second); time.Now().Before(until); {
			if err := os.WriteFile(os.Args[2], []byte(strconv.FormatInt(time.Now().UnixNano(), 10)), 0600); err != nil {
				return
			}
			time.Sleep(15 * time.Millisecond)
		}
	default:
		os.Exit(2)
	}
}
