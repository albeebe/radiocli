// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package weather

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/device"
)

// fakeConn is a device.Conn that answers each command from a function the test
// supplies, so the command can be driven with no scanner attached.
type fakeConn struct {
	reply func(command string) (string, error)
}

// Info describes the connected scanner.
func (f fakeConn) Info() device.Info { return device.Info{} }

// Execute answers the command the way the test asked it to.
func (f fakeConn) Execute(ctx context.Context, command string) (string, error) {
	return f.reply(command)
}

// ExecuteXML answers the command the way the test asked it to.
func (f fakeConn) ExecuteXML(ctx context.Context, command string) (string, error) {
	return f.reply(command)
}

// Send reports whatever answering the command would have reported.
func (f fakeConn) Send(ctx context.Context, command string) error {
	_, err := f.reply(command)
	return err
}

// Close releases nothing, because there is no port.
func (f fakeConn) Close() error { return nil }

// failWriter is a stream that refuses everything written to it, which is how a
// closed pipe behaves when the output is being read by something else.
type failWriter struct{}

// Write refuses the bytes and says why.
func (failWriter) Write(p []byte) (int, error) { return 0, errors.New("the pipe closed") }

// quiet returns an application context writing to buffers rather than to the
// terminal, along with the buffers it writes to.
func quiet() (*appcontext.App, *bytes.Buffer, *bytes.Buffer) {
	app := appcontext.New()
	out, notes := &bytes.Buffer{}, &bytes.Buffer{}
	app.Stdout, app.Stderr = out, notes
	return app, out, notes
}

// gsi builds the document the scanner answers "GSI" with, carrying the weather
// elements this command reads.
func gsi(mode, number, frequency, hold, rssi string) string {
	return fmt.Sprintf(`<ScannerInfo Mode="WX Scan" V_Screen="wx_alert">`+
		`<Property Rssi="%s"/><WxMode Mode="%s"/>`+
		`<WxChannel CH_No="%s" Freq="%s" Hold="%s"/></ScannerInfo>`,
		rssi, mode, number, frequency, hold)
}

// fast shortens the waits the sweep and the settle spend sitting out real time,
// so a test exercises the same number of polls in milliseconds rather than
// seconds. The originals are put back when the test finishes, so nothing leaks
// into another one.
func fast(t *testing.T) {
	t.Helper()

	oldDwell, oldDwellGap, oldSettleGap, oldTurnGap := dwell, dwellGap, settleGap, turnGap
	t.Cleanup(func() {
		dwell, dwellGap, settleGap, turnGap = oldDwell, oldDwellGap, oldSettleGap, oldTurnGap
	})

	// The ratio is what decides how many readings a dwell takes, so it is kept
	// at five the way the real pair is.
	dwell = 5 * time.Millisecond
	dwellGap = time.Millisecond
	settleGap = time.Millisecond
	turnGap = time.Millisecond
}

// TestNew covers the command New builds, including the closure it hands cobra.
//
// Coverage: 100% (3 test cases covering the function's only path)
//
// Test cases:
//   - Wiring: the command is named and carries its help text
//   - HasStop: the stop subcommand is attached
//   - Runs: executing the command reaches the scanner and reports its refusal
func TestNew(t *testing.T) {
	// Verify the command carries the name and the help text
	t.Run("Wiring", func(t *testing.T) {
		cmd := New(appcontext.New())

		if cmd.Use != "weather" {
			t.Errorf("the command is %q, wanted %q", cmd.Use, "weather")
		}
		if cmd.Short == "" || cmd.Long == "" {
			t.Error("the command has no help text")
		}
	})

	// Verify the way back out is reachable as a subcommand
	t.Run("HasStop", func(t *testing.T) {
		var found bool
		for _, sub := range New(appcontext.New()).Commands() {
			if sub.Name() == "stop" {
				found = true
			}
		}
		if !found {
			t.Error("the stop subcommand is not attached")
		}
	})

	// Verify executing the command runs the closure cobra was handed
	t.Run("Runs", func(t *testing.T) {
		app, _, _ := quiet()
		app.SetDevice(device.New(fakeConn{reply: func(command string) (string, error) {
			if strings.HasPrefix(command, "MNU,") {
				return "", errors.New("the port is gone")
			}
			return "", nil
		}}))

		cmd := New(app)
		cmd.SetArgs([]string{})
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})

		err := cmd.ExecuteContext(context.Background())
		if err == nil || !strings.Contains(err.Error(), "opening the weather menu") {
			t.Fatalf("the command reported %v, wanted the menu to be blamed", err)
		}
	})
}

// Test_newStop covers the stop subcommand and the closure it hands cobra.
//
// Coverage: 100% (2 test cases covering the function's only path)
//
// Test cases:
//   - Wiring: the subcommand is named and carries its help text
//   - Runs: executing it reads the scanner and reports it is not on weather
func Test_newStop(t *testing.T) {
	// Verify the subcommand carries the name and the help text
	t.Run("Wiring", func(t *testing.T) {
		cmd := newStop(appcontext.New())

		if cmd.Use != "stop" {
			t.Errorf("the subcommand is %q, wanted %q", cmd.Use, "stop")
		}
		if cmd.Short == "" || cmd.Long == "" {
			t.Error("the subcommand has no help text")
		}
	})

	// Verify executing the subcommand runs the closure cobra was handed
	t.Run("Runs", func(t *testing.T) {
		app, _, notes := quiet()
		app.SetDevice(device.New(fakeConn{reply: func(command string) (string, error) {
			return gsi("", "", "", "", "-999"), nil
		}}))

		cmd := newStop(app)
		cmd.SetArgs([]string{})
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})

		if err := cmd.ExecuteContext(context.Background()); err != nil {
			t.Fatalf("running the subcommand: %v", err)
		}
		if !strings.Contains(notes.String(), "not on the weather channels") {
			t.Errorf("the note was %q, wanted it to say the scanner is not on weather", notes)
		}
	})
}

// Test_describe tests the describe function with 100% coverage.
//
// Coverage: 100% (2 test cases covering both sides of the ordering)
//
// Test cases:
//   - ChannelOrder: the sweep is reordered by channel number and the held one
//     is marked
//   - UnnumberedChannels: names that are not numbers fall back to comparing
//     them as text
func Test_describe(t *testing.T) {
	// Verify the sweep comes back in channel order with the selection marked
	t.Run("ChannelOrder", func(t *testing.T) {
		strength := -97
		got := describe([]measurement{
			{Number: "7", Frequency: "162.525000MHz", Signal: &strength},
			{Number: "1", Frequency: "162.400000MHz"},
		}, "7")

		if len(got) != 2 {
			t.Fatalf("describe returned %d channels, wanted 2", len(got))
		}
		if got[0].Number != "1" || got[1].Number != "7" {
			t.Errorf("the channels came back as %q then %q, wanted 1 then 7",
				got[0].Number, got[1].Number)
		}
		if got[0].Selected {
			t.Error("channel 1 is marked as the one being held")
		}
		if !got[1].Selected {
			t.Error("channel 7 is not marked as the one being held")
		}
	})

	// Verify a channel the scanner did not number is still ordered somewhere
	t.Run("UnnumberedChannels", func(t *testing.T) {
		got := describe([]measurement{{Number: "b"}, {Number: "a"}}, "")

		if got[0].Number != "a" || got[1].Number != "b" {
			t.Errorf("the channels came back as %q then %q, wanted a then b",
				got[0].Number, got[1].Number)
		}
	})
}

// Test_enterMonitor tests the enterMonitor function with 100% coverage.
//
// Coverage: 100% (5 test cases covering every branch)
//
// Test cases:
//   - Monitor: the scanner lands in the mode that plays the broadcast
//   - MenuError: the weather menu could not be opened
//   - SelectError: the entry could not be chosen
//   - SettleError: the scanner never said what it was doing
//   - WeatherAlert: the scanner started the silent standby instead
//   - UnknownMode: the scanner reported a mode that is neither of them
func Test_enterMonitor(t *testing.T) {
	// Verify landing in Monitor Weather is accepted
	t.Run("Monitor", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			switch {
			case command == "STS":
				return "0,Weather Scan,****************", nil
			case command == "GSI":
				return gsi(monitorWeather, "1", "162.400000MHz", "Off", "-97"), nil
			}
			return "", nil
		}})

		if err := enterMonitor(context.Background(), client); err != nil {
			t.Fatalf("entering the broadcast mode: %v", err)
		}
	})

	// Verify a menu that will not open is reported
	t.Run("MenuError", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			if strings.HasPrefix(command, "MNU,") {
				return "", errors.New("the port is gone")
			}
			return "", nil
		}})

		err := enterMonitor(context.Background(), client)
		if err == nil || !strings.Contains(err.Error(), "opening the weather menu") {
			t.Fatalf("enterMonitor reported %v, wanted the menu to be blamed", err)
		}
	})

	// Verify an entry that cannot be chosen is reported
	t.Run("SelectError", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			if command == "STS" {
				return "", errors.New("the scanner refused the key")
			}
			return "", nil
		}})

		err := enterMonitor(context.Background(), client)
		if err == nil || !strings.Contains(err.Error(), `choosing "Weather Scan"`) {
			t.Fatalf("enterMonitor reported %v, wanted the entry to be blamed", err)
		}
	})

	// Verify a scanner that never reports a weather mode is reported
	t.Run("SettleError", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		client := device.New(fakeConn{reply: func(command string) (string, error) {
			switch command {
			case "STS":
				return "0,Weather Scan,****************", nil
			case "GSI":
				// Give up on the scanner rather than waiting out the whole
				// budget, which is the same branch at a fraction of the cost.
				cancel()
				return gsi("", "", "", "", "-999"), nil
			}
			return "", nil
		}})

		if err := enterMonitor(ctx, client); err == nil {
			t.Fatal("enterMonitor accepted a scanner that never reported a weather mode")
		}
	})

	// Verify the silent standby is caught rather than passed off as success
	t.Run("WeatherAlert", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			switch command {
			case "STS":
				return "0,Weather Scan,****************", nil
			case "GSI":
				return gsi(weatherAlert, "1", "162.400000MHz", "Off", "-97"), nil
			}
			return "", nil
		}})

		err := enterMonitor(context.Background(), client)
		if err == nil || !strings.Contains(err.Error(), "sitting on a weather channel silent") {
			t.Fatalf("enterMonitor reported %v, wanted the silent standby to be named", err)
		}
	})

	// Verify a mode belonging to neither is reported rather than assumed
	t.Run("UnknownMode", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			switch command {
			case "STS":
				return "0,Weather Scan,****************", nil
			case "GSI":
				return gsi("Something Else", "1", "162.400000MHz", "Off", "-97"), nil
			}
			return "", nil
		}})

		err := enterMonitor(context.Background(), client)
		if err == nil || !strings.Contains(err.Error(), "which is neither") {
			t.Fatalf("enterMonitor reported %v, wanted the unknown mode to be named", err)
		}
	})
}

// Test_holding tests the holding function with 100% coverage.
//
// Coverage: 100% (2 test cases covering both words it returns)
//
// Test cases:
//   - Held: a parked scanner is described as holding
//   - Moving: a scanner working through the channels is not
func Test_holding(t *testing.T) {
	// Verify a parked scanner is worded as holding
	t.Run("Held", func(t *testing.T) {
		info := device.ScannerInfo{Weather: device.Weather{
			Channel: device.WeatherChannel{Held: held},
		}}

		if got := holding(info); got != "holding" {
			t.Errorf("holding said %q, wanted %q", got, "holding")
		}
	})

	// Verify a moving scanner is worded as not holding
	t.Run("Moving", func(t *testing.T) {
		if got := holding(device.ScannerInfo{}); got != "not holding" {
			t.Errorf("holding said %q, wanted %q", got, "not holding")
		}
	})
}

// Test_measure tests the measure function with 100% coverage.
//
// Coverage: 100% (4 test cases covering every branch)
//
// Test cases:
//   - Strongest: the best of several readings on one channel is kept
//   - ReadError: a failed exchange is reported
//   - Cancelled: a cancelled context stops the dwell
//   - NoChannel: a scanner that never names a channel is reported
func Test_measure(t *testing.T) {
	fast(t)

	// Verify the strongest reading taken during the dwell is the one kept
	t.Run("Strongest", func(t *testing.T) {
		readings := []string{"-999", "-104", "-90", "-97", "-97"}
		var taken int
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			if command != "GSI" {
				return "", nil
			}
			rssi := readings[taken%len(readings)]
			taken++
			return gsi(monitorWeather, "3", "162.475000MHz", held, rssi), nil
		}})

		m, err := measure(context.Background(), client)
		if err != nil {
			t.Fatalf("measuring the channel: %v", err)
		}
		if m.Number != "3" || m.Frequency != "162.475000MHz" {
			t.Errorf("measure reported channel %q on %q, wanted 3 on 162.475000MHz",
				m.Number, m.Frequency)
		}
		if m.Signal == nil || *m.Signal != -90 {
			t.Errorf("measure kept %v, wanted the strongest reading of -90", m.Signal)
		}
	})

	// Verify a scanner that cannot be read is reported
	t.Run("ReadError", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			return "", errors.New("the port is gone")
		}})

		_, err := measure(context.Background(), client)
		if err == nil || !strings.Contains(err.Error(), "reading a weather channel") {
			t.Fatalf("measure reported %v, wanted the read to be blamed", err)
		}
	})

	// Verify a cancelled context ends the dwell rather than sitting it out
	t.Run("Cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		client := device.New(fakeConn{reply: func(command string) (string, error) {
			cancel()
			return gsi(monitorWeather, "3", "162.475000MHz", held, "-97"), nil
		}})

		if _, err := measure(ctx, client); !errors.Is(err, context.Canceled) {
			t.Fatalf("measure reported %v, wanted the cancellation", err)
		}
	})

	// Verify a scanner that names no channel at all is reported
	t.Run("NoChannel", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			return gsi(monitorWeather, "", "", "", "-999"), nil
		}})

		_, err := measure(context.Background(), client)
		if err == nil || !strings.Contains(err.Error(), "would not say which weather channel") {
			t.Fatalf("measure reported %v, wanted the silence to be named", err)
		}
	})
}

// Test_renderReport tests the renderReport function with 100% coverage.
//
// Coverage: 100% (6 test cases covering every branch)
//
// Test cases:
//   - JSON: the report is encoded as JSON
//   - JSONError: a stream that refuses the encoding is reported
//   - Off: a scanner that is not on the weather channels prints one line
//   - Receiving: the held channel and the sweep are tabulated
//   - NotReceiving: nothing receivable is said rather than shown as a channel
//   - FlushError: a stream that refuses the table is reported
func Test_renderReport(t *testing.T) {
	// Verify JSON mode encodes the report rather than tabulating it
	t.Run("JSON", func(t *testing.T) {
		app, out, _ := quiet()
		app.Config.Output = appcontext.OutputJSON
		strength := -97

		err := renderReport(app, report{
			Scanning: true, Mode: monitorWeather, Receiving: true,
			Channel: "7", Frequency: "162.525000MHz", Signal: &strength,
			Channels: []channel{{Number: "7", Frequency: "162.525000MHz", Signal: &strength, Selected: true}},
		})
		if err != nil {
			t.Fatalf("rendering the report: %v", err)
		}
		if !strings.Contains(out.String(), `"signal": -97`) {
			t.Errorf("the JSON was %q, wanted the signal in it", out)
		}
	})

	// Verify a stream that refuses the encoding is reported
	t.Run("JSONError", func(t *testing.T) {
		app, _, _ := quiet()
		app.Config.Output = appcontext.OutputJSON
		app.Stdout = failWriter{}

		if err := renderReport(app, report{}); err == nil {
			t.Fatal("renderReport accepted a stream that refuses everything")
		}
	})

	// Verify a scanner that is not on the weather channels says so
	t.Run("Off", func(t *testing.T) {
		app, out, _ := quiet()

		if err := renderReport(app, report{}); err != nil {
			t.Fatalf("rendering the report: %v", err)
		}
		if got, want := out.String(), "weather: off\n"; got != want {
			t.Errorf("renderReport wrote %q, wanted %q", got, want)
		}
	})

	// Verify the held channel and the evidence for choosing it are printed
	t.Run("Receiving", func(t *testing.T) {
		app, out, _ := quiet()
		strength := -97

		err := renderReport(app, report{
			Scanning: true, Mode: monitorWeather, Receiving: true,
			Channel: "7", Frequency: "162.525000MHz", Signal: &strength,
			Channels: []channel{
				{Number: "1", Frequency: "162.400000MHz"},
				{Number: "7", Frequency: "162.525000MHz", Signal: &strength, Selected: true},
			},
		})
		if err != nil {
			t.Fatalf("rendering the report: %v", err)
		}

		printed := out.String()
		for _, want := range []string{"Monitor Weather", "-97 dBm", "CHANNEL", "holding", "-"} {
			if !strings.Contains(printed, want) {
				t.Errorf("renderReport wrote %q, wanted %q in it", printed, want)
			}
		}
	})

	// Verify a sweep that heard nothing says so instead of naming a channel
	t.Run("NotReceiving", func(t *testing.T) {
		app, out, _ := quiet()

		err := renderReport(app, report{Scanning: true, Mode: monitorWeather})
		if err != nil {
			t.Fatalf("rendering the report: %v", err)
		}
		if !strings.Contains(out.String(), "none receivable") {
			t.Errorf("renderReport wrote %q, wanted it to say nothing is receivable", out)
		}
	})

	// Verify a stream that refuses the table is reported
	t.Run("FlushError", func(t *testing.T) {
		app, _, _ := quiet()
		app.Stdout = failWriter{}

		err := renderReport(app, report{
			Scanning: true, Mode: monitorWeather, Receiving: true,
			Channels: []channel{{Number: "1", Frequency: "162.400000MHz"}},
		})
		if err == nil {
			t.Fatal("renderReport accepted a stream that refuses everything")
		}
	})
}

// Test_runStart tests the runStart function with 100% coverage.
//
// Coverage: 100% (7 test cases covering every branch)
//
// Test cases:
//   - Success: the sweep runs and the scanner is parked on the best channel
//   - NoDevice: no scanner was named
//   - EnterError: the broadcast mode could not be started
//   - HoldError: the scanner could not be held before measuring
//   - SweepError: a channel could not be measured
//   - NothingHeard: no channel answered, so the scanner is let go again
//   - ReleaseError: the hold could not be released after hearing nothing
//   - ReturnError: the winning channel could not be returned to
func Test_runStart(t *testing.T) {
	fast(t)

	// Verify a full run measures every channel and parks on the strongest
	t.Run("Success", func(t *testing.T) {
		app, out, _ := quiet()
		app.Config.Output = appcontext.OutputJSON

		signals := []string{"-104", "-110", "-999", "-90", "-97", "-108", "-101"}
		channel := 0
		app.SetDevice(device.New(fakeConn{reply: func(command string) (string, error) {
			switch {
			case command == "STS":
				return "0,Weather Scan,****************", nil
			case command == "GSI":
				return gsi(monitorWeather, fmt.Sprint(channel+1),
					fmt.Sprintf("162.4%02d000MHz", channel), held, signals[channel]), nil
			case strings.HasPrefix(command, "KEY,>"):
				channel = (channel + 1) % channelCount
				return "", nil
			}
			return "", nil
		}}))

		if err := runStart(context.Background(), app); err != nil {
			t.Fatalf("running the command: %v", err)
		}

		printed := out.String()
		if !strings.Contains(printed, `"channel": "4"`) {
			t.Errorf("the report was %q, wanted the scanner parked on channel 4", printed)
		}
		if !strings.Contains(printed, `"signal": -90`) {
			t.Errorf("the report was %q, wanted the winning signal in it", printed)
		}
	})

	// Verify a run with no scanner named is refused before anything is pressed
	t.Run("NoDevice", func(t *testing.T) {
		app, _, _ := quiet()

		if err := runStart(context.Background(), app); err == nil {
			t.Fatal("runStart ran with no scanner named")
		}
	})

	// Verify a broadcast mode that will not start is reported
	t.Run("EnterError", func(t *testing.T) {
		app, _, _ := quiet()
		app.SetDevice(device.New(fakeConn{reply: func(command string) (string, error) {
			if strings.HasPrefix(command, "MNU,") {
				return "", errors.New("the port is gone")
			}
			return "", nil
		}}))

		err := runStart(context.Background(), app)
		if err == nil || !strings.Contains(err.Error(), "opening the weather menu") {
			t.Fatalf("runStart reported %v, wanted the menu to be blamed", err)
		}
	})

	// Verify a scanner that cannot be held is reported before measuring starts
	t.Run("HoldError", func(t *testing.T) {
		app, _, _ := quiet()
		var entered bool
		app.SetDevice(device.New(fakeConn{reply: func(command string) (string, error) {
			switch {
			case command == "STS":
				return "0,Weather Scan,****************", nil
			case command == "GSI":
				if entered {
					return "", errors.New("the port is gone")
				}
				entered = true
				return gsi(monitorWeather, "1", "162.400000MHz", "Off", "-97"), nil
			}
			return "", nil
		}}))

		err := runStart(context.Background(), app)
		if err == nil || !strings.Contains(err.Error(), "whether the scanner is holding") {
			t.Fatalf("runStart reported %v, wanted the hold to be blamed", err)
		}
	})

	// Verify a channel that cannot be measured ends the run
	t.Run("SweepError", func(t *testing.T) {
		app, _, _ := quiet()
		var reads int
		app.SetDevice(device.New(fakeConn{reply: func(command string) (string, error) {
			switch {
			case command == "STS":
				return "0,Weather Scan,****************", nil
			case command == "GSI":
				reads++
				if reads > 2 {
					return "", errors.New("the port is gone")
				}
				return gsi(monitorWeather, "1", "162.400000MHz", held, "-97"), nil
			}
			return "", nil
		}}))

		err := runStart(context.Background(), app)
		if err == nil || !strings.Contains(err.Error(), "reading a weather channel") {
			t.Fatalf("runStart reported %v, wanted the sweep to be blamed", err)
		}
	})

	// Verify hearing nothing anywhere lets the scanner go rather than parking it
	t.Run("NothingHeard", func(t *testing.T) {
		app, out, notes := quiet()
		app.Config.Output = appcontext.OutputJSON

		hold := held
		app.SetDevice(device.New(fakeConn{reply: func(command string) (string, error) {
			switch {
			case command == "STS":
				return "0,Weather Scan,****************", nil
			case command == "GSI":
				return gsi(monitorWeather, "1", "162.400000MHz", hold, "-999"), nil
			case strings.HasPrefix(command, "KEY,C"):
				hold = "Off"
				return "", nil
			}
			return "", nil
		}}))

		if err := runStart(context.Background(), app); err != nil {
			t.Fatalf("running the command: %v", err)
		}
		if !strings.Contains(notes.String(), "can be heard from here") {
			t.Errorf("the note was %q, wanted it to say nothing was heard", notes)
		}
		if !strings.Contains(out.String(), `"receiving": false`) {
			t.Errorf("the report was %q, wanted it to report nothing received", out)
		}
	})

	// Verify a hold that cannot be released after hearing nothing is reported
	t.Run("ReleaseError", func(t *testing.T) {
		app, _, _ := quiet()
		app.SetDevice(device.New(fakeConn{reply: func(command string) (string, error) {
			switch {
			case command == "STS":
				return "0,Weather Scan,****************", nil
			case command == "GSI":
				return gsi(monitorWeather, "1", "162.400000MHz", held, "-999"), nil
			case strings.HasPrefix(command, "KEY,C"):
				return "", errors.New("the scanner refused the key")
			}
			return "", nil
		}}))

		err := runStart(context.Background(), app)
		if err == nil || !strings.Contains(err.Error(), "pressing the hold key") {
			t.Fatalf("runStart reported %v, wanted the hold key to be blamed", err)
		}
	})

	// Verify failing to get back to the winning channel is reported
	t.Run("ReturnError", func(t *testing.T) {
		app, _, _ := quiet()
		var turns int
		app.SetDevice(device.New(fakeConn{reply: func(command string) (string, error) {
			switch {
			case command == "STS":
				return "0,Weather Scan,****************", nil
			case command == "GSI":
				if turns >= channelCount {
					return "", errors.New("the port is gone")
				}
				return gsi(monitorWeather, fmt.Sprint(turns+1), "162.400000MHz", held, "-97"), nil
			case strings.HasPrefix(command, "KEY,>"):
				turns++
				return "", nil
			}
			return "", nil
		}}))

		err := runStart(context.Background(), app)
		if err == nil || !strings.Contains(err.Error(), "which weather channel the scanner is on") {
			t.Fatalf("runStart reported %v, wanted the return walk to be blamed", err)
		}
	})
}

// Test_runStop tests the runStop function with 100% coverage.
//
// Coverage: 100% (6 test cases covering every branch)
//
// Test cases:
//   - Success: the scanner leaves the weather channels and scans again
//   - NoDevice: no scanner was named
//   - NotOnWeather: a scanner that was never there is left alone
//   - InfoError: the scanner could not be asked what it is doing
//   - JumpError: the scanner refused to go back to scanning
//   - SettleError: the scanner never left the weather channels
func Test_runStop(t *testing.T) {
	// Verify the scanner is jumped back to scanning and confirmed to have gone
	t.Run("Success", func(t *testing.T) {
		app, _, notes := quiet()

		var jumped bool
		app.SetDevice(device.New(fakeConn{reply: func(command string) (string, error) {
			switch {
			case strings.HasPrefix(command, "JPM,"):
				jumped = true
				return "", nil
			case command == "GSI":
				if jumped {
					return gsi("", "", "", "", "-999"), nil
				}
				return gsi(monitorWeather, "7", "162.525000MHz", held, "-97"), nil
			}
			return "", nil
		}}))

		if err := runStop(context.Background(), app); err != nil {
			t.Fatalf("stopping the weather channels: %v", err)
		}
		if !strings.Contains(notes.String(), "is scanning again") {
			t.Errorf("the note was %q, wanted it to say the scanner is scanning", notes)
		}
	})

	// Verify a run with no scanner named is refused
	t.Run("NoDevice", func(t *testing.T) {
		app, _, _ := quiet()

		if err := runStop(context.Background(), app); err == nil {
			t.Fatal("runStop ran with no scanner named")
		}
	})

	// Verify a scanner that is not on the weather channels is left alone
	t.Run("NotOnWeather", func(t *testing.T) {
		app, out, notes := quiet()
		var jumped bool
		app.SetDevice(device.New(fakeConn{reply: func(command string) (string, error) {
			if strings.HasPrefix(command, "JPM,") {
				jumped = true
			}
			return gsi("", "", "", "", "-999"), nil
		}}))

		if err := runStop(context.Background(), app); err != nil {
			t.Fatalf("stopping the weather channels: %v", err)
		}
		if jumped {
			t.Error("runStop moved a scanner that was not on the weather channels")
		}
		if !strings.Contains(notes.String(), "not on the weather channels") {
			t.Errorf("the note was %q, wanted it to say the scanner is not on weather", notes)
		}
		if got, want := out.String(), "weather: off\n"; got != want {
			t.Errorf("runStop wrote %q, wanted %q", got, want)
		}
	})

	// Verify a scanner that cannot be read is reported
	t.Run("InfoError", func(t *testing.T) {
		app, _, _ := quiet()
		app.SetDevice(device.New(fakeConn{reply: func(command string) (string, error) {
			return "", errors.New("the port is gone")
		}}))

		err := runStop(context.Background(), app)
		if err == nil || !strings.Contains(err.Error(), "what it is doing") {
			t.Fatalf("runStop reported %v, wanted the read to be blamed", err)
		}
	})

	// Verify a refused jump back to scanning is reported
	t.Run("JumpError", func(t *testing.T) {
		app, _, _ := quiet()
		app.SetDevice(device.New(fakeConn{reply: func(command string) (string, error) {
			if strings.HasPrefix(command, "JPM,") {
				return "", errors.New("the scanner refused the key")
			}
			return gsi(monitorWeather, "7", "162.525000MHz", held, "-97"), nil
		}}))

		err := runStop(context.Background(), app)
		if err == nil || !strings.Contains(err.Error(), "returning the scanner to scanning") {
			t.Fatalf("runStop reported %v, wanted the jump to be blamed", err)
		}
	})

	// Verify a scanner that stays on the weather channels is reported
	t.Run("SettleError", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		app, _, _ := quiet()
		var jumped bool
		app.SetDevice(device.New(fakeConn{reply: func(command string) (string, error) {
			if strings.HasPrefix(command, "JPM,") {
				jumped = true
				return "", nil
			}
			if jumped {
				// Give up rather than waiting out the whole settle budget,
				// which reaches the same branch far more cheaply.
				cancel()
			}
			return gsi(monitorWeather, "7", "162.525000MHz", held, "-97"), nil
		}}))

		if err := runStop(ctx, app); err == nil {
			t.Fatal("runStop accepted a scanner that never left the weather channels")
		}
	})
}

// Test_setHold tests the setHold function with 100% coverage.
//
// Coverage: 100% (5 test cases covering every branch)
//
// Test cases:
//   - AlreadyHolding: a scanner already in the wanted state is not pressed
//   - Holds: a scanner that is not holding is pressed and confirmed
//   - InfoError: the scanner could not be asked whether it is holding
//   - PressError: the hold key could not be pressed
//   - SettleError: the scanner did not end up the way it was asked to
func Test_setHold(t *testing.T) {
	// Verify a scanner already holding is left alone rather than toggled off
	t.Run("AlreadyHolding", func(t *testing.T) {
		var pressed bool
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			if strings.HasPrefix(command, "KEY,") {
				pressed = true
			}
			return gsi(monitorWeather, "1", "162.400000MHz", held, "-97"), nil
		}})

		if err := setHold(context.Background(), client, true); err != nil {
			t.Fatalf("holding the channel: %v", err)
		}
		if pressed {
			t.Error("setHold pressed the hold key on a scanner that was already holding")
		}
	})

	// Verify a scanner that is not holding is pressed and then confirmed
	t.Run("Holds", func(t *testing.T) {
		hold := "Off"
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			if strings.HasPrefix(command, "KEY,") {
				hold = held
				return "", nil
			}
			return gsi(monitorWeather, "1", "162.400000MHz", hold, "-97"), nil
		}})

		if err := setHold(context.Background(), client, true); err != nil {
			t.Fatalf("holding the channel: %v", err)
		}
		if hold != held {
			t.Errorf("the scanner is %q, wanted %q", hold, held)
		}
	})

	// Verify a scanner that cannot be read is reported
	t.Run("InfoError", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			return "", errors.New("the port is gone")
		}})

		err := setHold(context.Background(), client, true)
		if err == nil || !strings.Contains(err.Error(), "whether the scanner is holding") {
			t.Fatalf("setHold reported %v, wanted the read to be blamed", err)
		}
	})

	// Verify a refused hold key is reported
	t.Run("PressError", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			if strings.HasPrefix(command, "KEY,") {
				return "", errors.New("the scanner refused the key")
			}
			return gsi(monitorWeather, "1", "162.400000MHz", "Off", "-97"), nil
		}})

		err := setHold(context.Background(), client, true)
		if err == nil || !strings.Contains(err.Error(), "pressing the hold key") {
			t.Fatalf("setHold reported %v, wanted the key to be blamed", err)
		}
	})

	// Verify a press that did not take is reported with what the scanner is doing
	t.Run("SettleError", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		var pressed bool
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			if strings.HasPrefix(command, "KEY,") {
				pressed = true
				return "", nil
			}
			if pressed {
				// Give up rather than waiting out the whole settle budget.
				cancel()
			}
			return gsi(monitorWeather, "1", "162.400000MHz", "Off", "-97"), nil
		}})

		err := setHold(ctx, client, true)
		if err == nil || !strings.Contains(err.Error(), "not holding a weather channel after pressing hold") {
			t.Fatalf("setHold reported %v, wanted the state after the press", err)
		}
	})
}

// Test_settle tests the settle function with 100% coverage.
//
// Coverage: 100% (3 test cases covering every branch)
//
// Test cases:
//   - Settles: the first reading that satisfies the wait is returned at once
//   - Cancelled: a cancelled context ends the wait
//   - GivesUp: the whole budget is spent, over readings that fail and readings
//     that simply are not what was wanted
func Test_settle(t *testing.T) {
	fast(t)

	// Verify a scanner already in the wanted state answers on the first look
	t.Run("Settles", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			return gsi(monitorWeather, "1", "162.400000MHz", held, "-97"), nil
		}})

		info, err := settle(context.Background(), client, func(info device.ScannerInfo) bool {
			return info.Weather.Mode == monitorWeather
		})
		if err != nil {
			t.Fatalf("waiting for the scanner: %v", err)
		}
		if info.Weather.Channel.Number != "1" {
			t.Errorf("settle returned channel %q, wanted 1", info.Weather.Channel.Number)
		}
	})

	// Verify a cancelled context ends the wait rather than spending the budget
	t.Run("Cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		client := device.New(fakeConn{reply: func(command string) (string, error) {
			cancel()
			return gsi("", "", "", "", "-999"), nil
		}})

		_, err := settle(ctx, client, func(device.ScannerInfo) bool { return false })
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("settle reported %v, wanted the cancellation", err)
		}
	})

	// Verify a scanner that never arrives is given up on, whether it answers
	// badly or answers with the wrong thing
	t.Run("GivesUp", func(t *testing.T) {
		var looks int
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			looks++
			if looks%2 == 0 {
				return "", errors.New("the port is gone")
			}
			return gsi(weatherAlert, "1", "162.400000MHz", held, "-97"), nil
		}})

		info, err := settle(context.Background(), client, func(info device.ScannerInfo) bool {
			return info.Weather.Mode == monitorWeather
		})
		if err == nil || !strings.Contains(err.Error(), "is still in") {
			t.Fatalf("settle reported %v, wanted it to give up", err)
		}
		if info.Weather.Mode != weatherAlert {
			t.Errorf("settle returned %q, wanted the last reading it took", info.Weather.Mode)
		}
	})
}

// Test_signal tests the signal function with 100% coverage.
//
// Coverage: 100% (3 test cases covering every branch)
//
// Test cases:
//   - Reading: a genuine reading is returned
//   - NothingHeard: the scanner's own sentinel is not a reading
//   - NotANumber: an unreadable strength is not a reading either
func Test_signal(t *testing.T) {
	// Verify a genuine reading comes back as one
	t.Run("Reading", func(t *testing.T) {
		rssi, ok := signal(device.ScannerInfo{Property: device.Property{RSSI: "-97"}})

		if !ok || rssi != -97 {
			t.Errorf("signal read %d %v, wanted -97 and true", rssi, ok)
		}
	})

	// Verify the sentinel the scanner uses for silence is not treated as weak
	t.Run("NothingHeard", func(t *testing.T) {
		if _, ok := signal(device.ScannerInfo{Property: device.Property{RSSI: "-999"}}); ok {
			t.Error("signal treated the scanner's silence as a reading")
		}
	})

	// Verify a strength that is not a number is not treated as a reading
	t.Run("NotANumber", func(t *testing.T) {
		if _, ok := signal(device.ScannerInfo{Property: device.Property{RSSI: ""}}); ok {
			t.Error("signal treated an unreadable strength as a reading")
		}
	})
}

// Test_stepTo tests the stepTo function with 100% coverage.
//
// Coverage: 100% (5 test cases covering every branch)
//
// Test cases:
//   - AlreadyThere: the scanner is already on the wanted channel
//   - Steps: the knob is turned until the wanted channel comes up
//   - InfoError: the scanner could not be asked which channel it is on
//   - TurnError: the knob could not be turned
//   - NeverArrives: a full turn round without finding it is reported
func Test_stepTo(t *testing.T) {
	fast(t)

	// Verify a scanner already on the channel is not turned at all
	t.Run("AlreadyThere", func(t *testing.T) {
		var turned bool
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			if strings.HasPrefix(command, "KEY,") {
				turned = true
			}
			return gsi(monitorWeather, "4", "162.425000MHz", held, "-90"), nil
		}})

		on, err := stepTo(context.Background(), client, "4")
		if err != nil {
			t.Fatalf("stepping to the channel: %v", err)
		}
		if on.Frequency != "162.425000MHz" {
			t.Errorf("stepTo reported %q, wanted 162.425000MHz", on.Frequency)
		}
		if turned {
			t.Error("stepTo turned the knob on a scanner that was already there")
		}
	})

	// Verify the knob is turned until the wanted channel comes up
	t.Run("Steps", func(t *testing.T) {
		channel := 1
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			if strings.HasPrefix(command, "KEY,") {
				channel++
				return "", nil
			}
			return gsi(monitorWeather, fmt.Sprint(channel), "162.400000MHz", held, "-97"), nil
		}})

		on, err := stepTo(context.Background(), client, "3")
		if err != nil {
			t.Fatalf("stepping to the channel: %v", err)
		}
		if on.Number != "3" {
			t.Errorf("stepTo stopped on %q, wanted 3", on.Number)
		}
	})

	// Verify a scanner that cannot be read is reported
	t.Run("InfoError", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			return "", errors.New("the port is gone")
		}})

		_, err := stepTo(context.Background(), client, "4")
		if err == nil || !strings.Contains(err.Error(), "which weather channel the scanner is on") {
			t.Fatalf("stepTo reported %v, wanted the read to be blamed", err)
		}
	})

	// Verify a knob that will not turn is reported
	t.Run("TurnError", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			if strings.HasPrefix(command, "KEY,") {
				return "", errors.New("the scanner refused the key")
			}
			return gsi(monitorWeather, "1", "162.400000MHz", held, "-97"), nil
		}})

		_, err := stepTo(context.Background(), client, "4")
		if err == nil || !strings.Contains(err.Error(), "turning the knob") {
			t.Fatalf("stepTo reported %v, wanted the knob to be blamed", err)
		}
	})

	// Verify a channel that never comes up is reported with what did
	t.Run("NeverArrives", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			if strings.HasPrefix(command, "KEY,") {
				return "", nil
			}
			return gsi(monitorWeather, "1", "162.400000MHz", held, "-97"), nil
		}})

		_, err := stepTo(context.Background(), client, "4")
		if err == nil || !strings.Contains(err.Error(), "could not get back to weather channel 4") {
			t.Fatalf("stepTo reported %v, wanted the missing channel to be named", err)
		}
	})
}

// Test_strength tests the strength function with 100% coverage.
//
// Coverage: 100% (2 test cases covering both words it returns)
//
// Test cases:
//   - Reading: a reading is worded with its unit
//   - NothingHeard: an absent reading is worded as absent rather than as zero
func Test_strength(t *testing.T) {
	// Verify a reading carries its unit
	t.Run("Reading", func(t *testing.T) {
		rssi := -97

		if got, want := strength(&rssi), "-97 dBm"; got != want {
			t.Errorf("strength said %q, wanted %q", got, want)
		}
	})

	// Verify nothing heard is not printed as a number
	t.Run("NothingHeard", func(t *testing.T) {
		if got, want := strength(nil), "-"; got != want {
			t.Errorf("strength said %q, wanted %q", got, want)
		}
	})
}

// Test_strongest tests the strongest function with 100% coverage.
//
// Coverage: 100% (3 test cases covering every branch)
//
// Test cases:
//   - Strongest: the best reading in the sweep wins
//   - Ties: the channel reached first wins a tie
//   - NothingHeard: a sweep that heard nothing picks nothing
func Test_strongest(t *testing.T) {
	// Verify the strongest reading wins wherever it sits in the sweep
	t.Run("Strongest", func(t *testing.T) {
		weak, strong := -108, -90

		got := strongest([]measurement{
			{Number: "1"},
			{Number: "2", Signal: &weak},
			{Number: "3", Signal: &strong},
		})
		if got != 2 {
			t.Errorf("strongest picked %d, wanted 2", got)
		}
	})

	// Verify a tie goes to the channel the sweep reached first
	t.Run("Ties", func(t *testing.T) {
		first, second := -97, -97

		got := strongest([]measurement{
			{Number: "1", Signal: &first},
			{Number: "2", Signal: &second},
		})
		if got != 0 {
			t.Errorf("strongest picked %d, wanted the first of the two", got)
		}
	})

	// Verify a sweep that heard nothing picks nothing at all
	t.Run("NothingHeard", func(t *testing.T) {
		if got := strongest([]measurement{{Number: "1"}, {Number: "2"}}); got != -1 {
			t.Errorf("strongest picked %d, wanted -1", got)
		}
	})
}

// Test_sweep tests the sweep function with 100% coverage.
//
// Coverage: 100% (3 test cases covering every branch)
//
// Test cases:
//   - EveryChannel: all seven channels are measured, in the order visited
//   - MeasureError: a channel that could not be read ends the sweep
//   - TurnError: a knob that will not turn ends the sweep
func Test_sweep(t *testing.T) {
	fast(t)

	// Verify the sweep visits every channel and comes back round
	t.Run("EveryChannel", func(t *testing.T) {
		channel := 0
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			if strings.HasPrefix(command, "KEY,") {
				channel = (channel + 1) % channelCount
				return "", nil
			}
			return gsi(monitorWeather, fmt.Sprint(channel+1), "162.400000MHz", held, "-97"), nil
		}})

		found, err := sweep(context.Background(), client)
		if err != nil {
			t.Fatalf("sweeping the channels: %v", err)
		}
		if len(found) != channelCount {
			t.Fatalf("the sweep found %d channels, wanted %d", len(found), channelCount)
		}
		if found[0].Number != "1" || found[channelCount-1].Number != fmt.Sprint(channelCount) {
			t.Errorf("the sweep ran %q to %q, wanted 1 to %d",
				found[0].Number, found[channelCount-1].Number, channelCount)
		}
		if channel != 0 {
			t.Errorf("the sweep finished on channel %d, wanted it back where it started", channel+1)
		}
	})

	// Verify a channel that cannot be measured ends the sweep
	t.Run("MeasureError", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			return "", errors.New("the port is gone")
		}})

		if _, err := sweep(context.Background(), client); err == nil {
			t.Fatal("the sweep accepted a scanner that could not be read")
		}
	})

	// Verify a knob that will not turn ends the sweep
	t.Run("TurnError", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			if strings.HasPrefix(command, "KEY,") {
				return "", errors.New("the scanner refused the key")
			}
			return gsi(monitorWeather, "1", "162.400000MHz", held, "-97"), nil
		}})

		_, err := sweep(context.Background(), client)
		if err == nil || !strings.Contains(err.Error(), "turning the knob") {
			t.Fatalf("the sweep reported %v, wanted the knob to be blamed", err)
		}
	})
}

// Test_turn tests the turn function with 100% coverage.
//
// Coverage: 100% (3 test cases covering every branch)
//
// Test cases:
//   - Turns: the knob is turned and the scanner given time to redraw
//   - PressError: a refused key is reported
//   - Cancelled: a cancelled context ends the wait after the turn
func Test_turn(t *testing.T) {
	fast(t)

	// Verify the knob is turned to the right
	t.Run("Turns", func(t *testing.T) {
		var sent string
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			if strings.HasPrefix(command, "KEY,") {
				sent = command
			}
			return "", nil
		}})

		if err := turn(context.Background(), client); err != nil {
			t.Fatalf("turning the knob: %v", err)
		}
		if !strings.HasPrefix(sent, "KEY,>") {
			t.Errorf("turn sent %q, wanted the knob turned to the right", sent)
		}
	})

	// Verify a refused key is reported
	t.Run("PressError", func(t *testing.T) {
		client := device.New(fakeConn{reply: func(command string) (string, error) {
			return "", errors.New("the scanner refused the key")
		}})

		err := turn(context.Background(), client)
		if err == nil || !strings.Contains(err.Error(), "turning the knob") {
			t.Fatalf("turn reported %v, wanted the knob to be blamed", err)
		}
	})

	// Verify a cancelled context ends the wait that follows the turn
	t.Run("Cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		client := device.New(fakeConn{reply: func(command string) (string, error) {
			return "", nil
		}})

		if err := turn(ctx, client); !errors.Is(err, context.Canceled) {
			t.Fatalf("turn reported %v, wanted the cancellation", err)
		}
	})
}
