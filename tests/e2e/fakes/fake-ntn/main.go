package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "doctor", "preflight", "status":
			fmt.Println(`{"status":"ok","provider":"notion"}`)
			return
		case "create", "update":
			fmt.Println(`{"status":"ok","id":"fake_notion_page","url":"https://example.test/notion/fake_notion_page"}`)
			return
		}
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
