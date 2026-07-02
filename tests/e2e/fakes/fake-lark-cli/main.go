package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) > 2 && os.Args[1] == "drive" && os.Args[2] == "+create-folder" {
		fmt.Println(`{"status":"ok","id":"fake_folder_token","url":"https://example.test/folder/fake_folder_token"}`)
		return
	}
	if len(os.Args) > 2 && os.Args[1] == "drive" && os.Args[2] == "+move" {
		fmt.Println(`{"status":"ok"}`)
		return
	}
	if len(os.Args) > 2 && os.Args[1] == "auth" && os.Args[2] == "status" {
		fmt.Println(`{"identities":{"user":{"status":"ready","available":true},"bot":{"status":"ready","available":true}}}`)
		return
	}
	if len(os.Args) > 2 && os.Args[1] == "markdown" && (os.Args[2] == "+create" || os.Args[2] == "+overwrite") {
		fmt.Println(`{"status":"ok","id":"fake_lark_doc","url":"https://example.test/lark/fake_lark_doc"}`)
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "status" {
		fmt.Println(`{"status":"ok","provider":"feishu"}`)
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

	fmt.Println(`{"mocked":true,"cli":"lark-cli"}`)
}
