package execobservable

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os/exec"
	"sync"
	"time"
)

//https://bountify.co/golang-parse-stdout

// CmdProgressCallback interface for doing callbacks
type CmdProgressCallback interface {
	Progress(string, SendResponse)
}

// SendResponse interface for sending responses
type SendResponse interface {
	Send(string)
}

// CmdRunner object for running commands
type CmdRunner struct {
	sendCh    chan string
	ExitState int
	ExitError error
}

// RunCommand runs a command with realtime feedback
func (runner *CmdRunner) RunCommand(command string, dir string, progress CmdProgressCallback, flags ...string) {
	wg := sync.WaitGroup{}
	wg.Add(1)
	fmt.Println("Starting command..")
	recv := make(chan string)
	runner.sendCh = make(chan string)
	go func() {
		err := runCommandCh(recv, runner.sendCh, "\r\n", command, dir, flags...)
		if err != nil {
			fmt.Printf("bad shit happneed %v\n", err)
			runner.ExitState = 1
			runner.ExitError = err
			//log.Fatal(err)
		} else {
			runner.ExitState = 0
			fmt.Println("All good")
		}
		fmt.Println("run thread complete")
		wg.Done()
	}()

	for v := range recv {
		if progress != nil {
			progress.Progress(v, runner)
		}
	}
	wg.Wait()
	println("RunCommands: done receiving from channel")
}

// Send implementation
func (runner *CmdRunner) Send(s string) {
	runner.sendCh <- s
}

// RunCommandCh runs an arbitrary command and streams output to a channnel.
func runCommandCh(stdoutCh chan<- string, stdinCh <-chan string, cutset string, command string, dir string, flags ...string) error {
	cmd := exec.Command(command, flags...)

	if dir != "" {
		cmd.Dir = dir
	}

	output, err := cmd.StdoutPipe()
	inputPipe, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("RunCommand: cmd.StdoutPipe(): %v", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		log.Fatal(err)
	}

	if err := cmd.Start(); err != nil {
		close(stdoutCh)
		return fmt.Errorf("RunCommand: cmd.Start(): %v", err)
	}

	go func() {
		//Potential bug.. with defer close, the error thrown if err != io.EOF will be eaten
		//defer close(stdoutCh)
		for {
			buf := make([]byte, 1024)
			n, err := output.Read(buf)
			if err != nil {
				//Todo check this error more specifically
				//Currently when the process that is being invoked exits, this generates an already closed error
				//hence the need for this specific check for that error
				_, found := err.(error)
				if found == true && n == 0 {
					break
				}

				if err != io.EOF {
					log.Fatalf("ReadError: %v", err)
				}
				if n == 0 {
					println("NoChars, exiting")
					break
				}
			}
			//text := string(buf[:n]) + "<-- chunk\n"
			text := string(buf[:n])
			stdoutCh <- text

			select {
			case res := <-stdinCh:
				fmt.Printf("got message on channel:%v\n", res)
				inputPipe.Write([]byte(res))
			case <-time.After(1 * time.Second):
				fmt.Println("timeout 1")
			}
		}
		fmt.Println("readthreadexit")
		close(stdoutCh) // closing the channel will cause parent process to exit
	}()
	slurp, _ := ioutil.ReadAll(stderr)

	println("Done receiving, starting wait")
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("RunCommand: cmd.Wait() returned error: %v %v", err, string(slurp))
	}

	// if len(slurp) == 0 {
	// 	fmt.Println("no error slurp")
	// } else {
	// 	//fmt.Printf("Error Slurp %s\n", slurp)
	// 	return fmt.Errorf("RunCommand: error slurp not empty %v", slurp)
	// }

	println("done waiting for exec to complete")
	return nil
}
