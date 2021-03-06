/*
Copyright 2011 Google Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// +build cgo

package tasks

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

func LookupGroupId(group string) (gid int, err error) {
	f, err := os.Open("/etc/group")
	if err != nil {
		return
	}
	defer f.Close()
	br := bufio.NewReader(f)
	for {
		s, err := br.ReadString('\n')
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, err
		}
		p := strings.Split(s, ":")
		if len(p) >= 3 && p[0] == group {
			return strconv.Atoi(p[2])
		}
	}
	return 0, fmt.Errorf("group %q not found", group)
}
