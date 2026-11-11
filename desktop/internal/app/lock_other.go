//go:build !windows

package app

import "fmt"

func acquireSingleInstance() (func(), bool, error) {
	return func() {}, false, nil
}

func notifyAlreadyRunning() {
	fmt.Println("拾光自习室已在运行")
}

func showError(caption, text string) {
	fmt.Printf("%s: %s\n", caption, text)
}

func showMessage(text, caption string) {
	fmt.Printf("%s: %s\n", caption, text)
}
