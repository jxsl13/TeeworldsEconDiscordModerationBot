package main

import "testing"

func Test_sendText(t *testing.T) {

	cmdQueue := make(chan command, 10)

	sendText(cmdQueue, "123456789012345678901234567890123456789012345678901234567890")
	cmd := <-cmdQueue

	if cmd.Command != "say 123456789012345678901234567890123456789012345678901234567890" {
		t.Fatal(cmd.Command)
	}

	sendText(cmdQueue, "this is some short text")
	cmd = <-cmdQueue

	if cmd.Command != "say this is some short text" {
		t.Fatal(cmd.Command)
	}

	sendText(cmdQueue, "this is some rather ultra super duper long long text that should have some unnecessary characters.")
	cmd = <-cmdQueue

	if cmd.Command != "say this is some rather ultra super duper long long text that" {
		t.Fatal(cmd.Command)
	}

	cmd = <-cmdQueue

	if cmd.Command != "say should have some unnecessary characters." {
		t.Fatal(cmd.Command)
	}

}
