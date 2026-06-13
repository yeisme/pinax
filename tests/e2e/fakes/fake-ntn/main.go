package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "status" {
		fmt.Println(`{"status":"ok","provider":"notion"}`)
		return
	}

	hasEvents := false
	for _, arg := range os.Args {
		if arg == "--events" {
			hasEvents = true
		}
	}

	if hasEvents {
		fmt.Println(`{"type":"start","seq":1}`)
		fmt.Println(`{"type":"event","seq":2,"data":{"synced":true}}`)
		fmt.Println(`{"type":"end","seq":3,"status":"success"}`)
		return
	}

	fmt.Println(`{"mocked":true,"cli":"ntn"}`)
}
