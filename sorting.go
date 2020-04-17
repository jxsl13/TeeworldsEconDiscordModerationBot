package main

type byName []string

func (a byName) Len() int           { return len(a) }
func (a byName) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a byName) Less(i, j int) bool { return a[i] < a[j] }

type byAddress []address

func (a byAddress) Len() int           { return len(a) }
func (a byAddress) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a byAddress) Less(i, j int) bool { return a[i] < a[j] }
