package main

type byName []string

func (a byName) Len() int           { return len(a) }
func (a byName) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a byName) Less(i, j int) bool { return a[i] < a[j] }

type byAddress []Address

func (a byAddress) Len() int           { return len(a) }
func (a byAddress) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a byAddress) Less(i, j int) bool { return a[i] < a[j] }

type byBantime []Ban

func (a byBantime) Len() int           { return len(a) }
func (a byBantime) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a byBantime) Less(i, j int) bool { return a[i].ExpiresAt.Before(a[j].ExpiresAt) }
