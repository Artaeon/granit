#!/bin/bash
sed -i 's/Title:    fmt.Sprintf("Demo Event %d", i),/Title:    fmt.Sprintf("Event scheduled for %d:00", i),/g' internal/tui/dailyplanner.go
sed -i 's/Name:   fmt.Sprintf("Habit %d", i),/Name:   "Read 30 mins",/g' internal/tui/dailyplanner.go

sed -i 's/color: "#7D56F4"/color: "#CBA6F7"/g' internal/tui/taskmanager.go
sed -i 's/color: "#43BF6D"/color: "#A6E3A1"/g' internal/tui/taskmanager.go

