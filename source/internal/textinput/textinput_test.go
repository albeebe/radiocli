// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/12/2026

package textinput

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/albeebe/radiocli/internal/device"
)

// stubConn is a fake device.Conn that answers the two exchanges this package
// makes: reading the menu with MSI, and pressing a key with KEY. A test
// supplies either behaviour as a function; an absent one answers emptily,
// which the scanner treats as success.
type stubConn struct {
	exec    func(command string) (string, error)
	execXML func(command string) (string, error)
}

// Info describes the fake scanner, which nothing here inspects.
func (c *stubConn) Info() device.Info { return device.Info{} }

// Execute answers a key press with whatever the test supplied.
func (c *stubConn) Execute(ctx context.Context, command string) (string, error) {
	if c.exec == nil {
		return "", nil
	}
	return c.exec(command)
}

// ExecuteXML answers a menu read with whatever the test supplied.
func (c *stubConn) ExecuteXML(ctx context.Context, command string) (string, error) {
	if c.execXML == nil {
		return "", nil
	}
	return c.execXML(command)
}

// Send is unused by this package and always succeeds.
func (c *stubConn) Send(ctx context.Context, command string) error { return nil }

// Close is unused by this package and always succeeds.
func (c *stubConn) Close() error { return nil }

// TestSet tests the Set function with 100% coverage.
//
// Coverage: 100% (10 test cases covering all branches)
//
// Test cases:
//   - MenuInfoError: a screen that cannot be read is reported
//   - NotTextEntry: being on the wrong kind of screen is refused
//   - CheckError: a value the screen cannot hold is refused before pressing
//   - NumericScreen: a screen that accepts only digits is typed on the keypad
//   - AlreadyCorrect: a value that is already right is accepted without editing
//   - CommitError: a refused acceptance is reported
//   - Success: each character is turned to and the value accepted
//   - LongerCurrent: the tail of a longer old value is blanked
//   - GrowsValue: a value longer than the old one is entered past its end
//   - SetCharError: a failure setting one character is passed on
//   - AdvanceError: a refused cursor move is passed on
func TestSet(t *testing.T) {
	// Builds the document the scanner answers a menu read with.
	menuDoc := func(menuType, value, enableKeys string, maxLength int) string {
		return fmt.Sprintf(`<MenuInfo Name="Name" MenuType="%s" Value="%s"><MenuInput MaxLength="%d" EnableKeys="%s"/></MenuInfo>`,
			menuType, value, maxLength, enableKeys)
	}

	// Verify that a screen that cannot be read is reported
	t.Run("MenuInfoError", func(t *testing.T) {
		conn := &stubConn{execXML: func(command string) (string, error) {
			return "", errors.New("the port is gone")
		}}

		err := Set(context.Background(), device.New(conn), "AB")
		if err == nil {
			t.Fatal("expected an error when the screen cannot be read, got none")
		}
		if !strings.Contains(err.Error(), "reading the text entry screen") {
			t.Errorf("expected the message to say it was reading the screen, got: %v", err)
		}
	})

	// Verify that being on a screen that is not for text entry is refused
	t.Run("NotTextEntry", func(t *testing.T) {
		conn := &stubConn{execXML: func(command string) (string, error) {
			return menuDoc("TypeSelect", "", "", 0), nil
		}}

		err := Set(context.Background(), device.New(conn), "AB")
		if err == nil {
			t.Fatal("expected an error when the scanner is on another screen, got none")
		}
		if !strings.Contains(err.Error(), "not on a text entry screen") {
			t.Errorf("expected the message to say it is not a text entry screen, got: %v", err)
		}
	})

	// Verify that a value the screen cannot hold is refused before anything is pressed
	t.Run("CheckError", func(t *testing.T) {
		pressed := false
		conn := &stubConn{
			execXML: func(command string) (string, error) {
				return menuDoc("TypeInput", "", "ABC ", 2), nil
			},
			exec: func(command string) (string, error) {
				pressed = true
				return "", nil
			},
		}

		if err := Set(context.Background(), device.New(conn), "ABCD"); err == nil {
			t.Fatal("expected an error for a value that is too long, got none")
		}
		if pressed {
			t.Error("expected nothing to be pressed when the value is refused")
		}
	})

	// Verify that a screen accepting only digits is typed on the keypad
	t.Run("NumericScreen", func(t *testing.T) {
		var keys []string
		typedValue := ""
		conn := &stubConn{
			execXML: func(command string) (string, error) {
				return menuDoc("TypeInput", typedValue, "0123456789.", 8), nil
			},
			exec: func(command string) (string, error) {
				keys = append(keys, command)

				// A digit key appends to the value, as the keypad does.
				if strings.HasPrefix(command, "KEY,") {
					key := strings.TrimSuffix(strings.TrimPrefix(command, "KEY,"), ",P")
					if strings.IndexByte(keypad, key[0]) >= 0 {
						typedValue += key
					}
				}
				return "", nil
			},
		}

		if err := Set(context.Background(), device.New(conn), "123"); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if typedValue != "123" {
			t.Errorf("expected the screen to hold \"123\", got %q", typedValue)
		}
		if keys[len(keys)-1] != "KEY,E,P" {
			t.Errorf("expected the value to be accepted last, got %q", keys[len(keys)-1])
		}
	})

	// Verify that a value that is already correct is accepted without editing
	t.Run("AlreadyCorrect", func(t *testing.T) {
		var keys []string
		conn := &stubConn{
			execXML: func(command string) (string, error) {
				return menuDoc("TypeInput", "AB", "ABC ", 8), nil
			},
			exec: func(command string) (string, error) {
				keys = append(keys, command)
				return "", nil
			},
		}

		if err := Set(context.Background(), device.New(conn), "AB"); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		// Nothing is edited, but the screen is still accepted.
		if len(keys) != 1 || keys[0] != "KEY,E,P" {
			t.Errorf("expected only the accept key to be pressed, got %v", keys)
		}
	})

	// Verify that a refused acceptance is reported
	t.Run("CommitError", func(t *testing.T) {
		conn := &stubConn{
			execXML: func(command string) (string, error) {
				return menuDoc("TypeInput", "AB", "ABC ", 8), nil
			},
			exec: func(command string) (string, error) {
				return "", errors.New("the scanner refused the key")
			},
		}

		err := Set(context.Background(), device.New(conn), "AB")
		if err == nil {
			t.Fatal("expected an error when the accept key is refused, got none")
		}
		if !strings.Contains(err.Error(), "pressing a key on the text entry screen") {
			t.Errorf("expected the message to say it was pressing a key, got: %v", err)
		}
	})

	// Verify that each character is turned to and the finished value accepted
	t.Run("Success", func(t *testing.T) {
		// A fake screen: the value it holds, where the cursor is, and the
		// characters the knob turns through.
		screen := []byte("AB")
		cursor := 0
		order := " ABC"
		committed := false

		conn := &stubConn{
			execXML: func(command string) (string, error) {
				return menuDoc("TypeInput", string(screen), "ABC ", 8), nil
			},
			exec: func(command string) (string, error) {
				key := strings.TrimSuffix(strings.TrimPrefix(command, "KEY,"), ",P")

				// Editing a position the value does not reach extends it, the
				// way the scanner's own screen does.
				grow := func() {
					for cursor >= len(screen) {
						screen = append(screen, ' ')
					}
				}

				switch key {
				case ">":
					grow()
					screen[cursor] = order[(strings.IndexByte(order, screen[cursor])+1)%len(order)]
				case "C":
					cursor++
				case ".":
					grow()
					screen[cursor] = ' '
				case "E":
					committed = true
				}
				return "", nil
			},
		}

		if err := Set(context.Background(), device.New(conn), "AC"); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if string(screen) != "AC" {
			t.Errorf("expected the screen to hold \"AC\", got %q", string(screen))
		}
		if !committed {
			t.Error("expected the value to be accepted")
		}
	})

	// Verify that the tail of a longer old value is blanked rather than left behind
	t.Run("LongerCurrent", func(t *testing.T) {
		screen := []byte("ABC")
		cursor := 0
		order := " ABC"

		conn := &stubConn{
			execXML: func(command string) (string, error) {
				return menuDoc("TypeInput", string(screen), "ABC ", 8), nil
			},
			exec: func(command string) (string, error) {
				key := strings.TrimSuffix(strings.TrimPrefix(command, "KEY,"), ",P")
				grow := func() {
					for cursor >= len(screen) {
						screen = append(screen, ' ')
					}
				}

				switch key {
				case ">":
					grow()
					screen[cursor] = order[(strings.IndexByte(order, screen[cursor])+1)%len(order)]
				case "C":
					cursor++
				case ".":
					grow()
					screen[cursor] = ' '
				}
				return "", nil
			},
		}

		if err := Set(context.Background(), device.New(conn), "A"); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if strings.TrimRight(string(screen), " ") != "A" {
			t.Errorf("expected the screen to hold \"A\" with the tail blanked, got %q", string(screen))
		}
	})

	// Verify that a value longer than the old one is entered past its end
	t.Run("GrowsValue", func(t *testing.T) {
		screen := []byte("A")
		cursor := 0
		order := " ABC"

		conn := &stubConn{
			execXML: func(command string) (string, error) {
				return menuDoc("TypeInput", string(screen), "ABC ", 8), nil
			},
			exec: func(command string) (string, error) {
				key := strings.TrimSuffix(strings.TrimPrefix(command, "KEY,"), ",P")
				grow := func() {
					for cursor >= len(screen) {
						screen = append(screen, ' ')
					}
				}

				switch key {
				case ">":
					grow()
					screen[cursor] = order[(strings.IndexByte(order, screen[cursor])+1)%len(order)]
				case "C":
					cursor++
				case ".":
					grow()
					screen[cursor] = ' '
				}
				return "", nil
			},
		}

		if err := Set(context.Background(), device.New(conn), "AB"); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if string(screen) != "AB" {
			t.Errorf("expected the screen to hold \"AB\", got %q", string(screen))
		}
	})

	// Verify that a failure setting one character is passed on
	t.Run("SetCharError", func(t *testing.T) {
		reads := 0
		conn := &stubConn{execXML: func(command string) (string, error) {
			reads++

			// The first read is the one Set makes itself; the next belongs to
			// setChar and fails.
			if reads == 1 {
				return menuDoc("TypeInput", "AB", "ABC ", 8), nil
			}
			return "", errors.New("the port is gone")
		}}

		if err := Set(context.Background(), device.New(conn), "CC"); err == nil {
			t.Error("expected an error when a character cannot be set, got none")
		}
	})

	// Verify that a refused cursor move is passed on
	t.Run("AdvanceError", func(t *testing.T) {
		conn := &stubConn{
			execXML: func(command string) (string, error) {
				return menuDoc("TypeInput", "AB", "ABC ", 8), nil
			},
			exec: func(command string) (string, error) {
				if command == "KEY,C,P" {
					return "", errors.New("the scanner refused the key")
				}
				return "", nil
			},
		}

		// The first character is already correct, so the first press made is
		// the one that moves the cursor.
		if err := Set(context.Background(), device.New(conn), "AC"); err == nil {
			t.Error("expected an error when the cursor cannot be moved, got none")
		}
	})
}

// Test_at tests the at function with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Within: a position the value reaches gives its byte
//   - Past: a position past the end reads as a space
func Test_at(t *testing.T) {
	// Verify that a position the value reaches gives the byte there
	t.Run("Within", func(t *testing.T) {
		if got := at("AB", 1); got != 'B' {
			t.Errorf("expected 'B', got %q", string(got))
		}
	})

	// Verify that a position past the end of the value reads as a space
	t.Run("Past", func(t *testing.T) {
		if got := at("AB", 5); got != ' ' {
			t.Errorf("expected a space, got %q", string(got))
		}
	})
}

// Test_charAt tests the charAt function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - Within: a position the value reaches is read from the screen
//   - PastEnd: a position past the end reports the space as a guess
//   - ReadError: a screen that cannot be read is reported
func Test_charAt(t *testing.T) {
	// Builds the document the scanner answers a menu read with.
	menuDoc := func(value string) string {
		return fmt.Sprintf(`<MenuInfo Name="Name" MenuType="TypeInput" Value="%s"><MenuInput MaxLength="8" EnableKeys="ABC "/></MenuInfo>`, value)
	}

	// Verify that a position the value reaches is read from the screen
	t.Run("Within", func(t *testing.T) {
		conn := &stubConn{execXML: func(command string) (string, error) {
			return menuDoc("AB"), nil
		}}

		got, real, err := charAt(context.Background(), device.New(conn), 1)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if got != 'B' || !real {
			t.Errorf("expected 'B' really held, got %q real=%v", string(got), real)
		}
	})

	// Verify that a position past the end reports its space as a guess
	t.Run("PastEnd", func(t *testing.T) {
		conn := &stubConn{execXML: func(command string) (string, error) {
			return menuDoc("AB"), nil
		}}

		got, real, err := charAt(context.Background(), device.New(conn), 5)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if got != ' ' || real {
			t.Errorf("expected a guessed space, got %q real=%v", string(got), real)
		}
	})

	// Verify that a screen that cannot be read is reported
	t.Run("ReadError", func(t *testing.T) {
		conn := &stubConn{execXML: func(command string) (string, error) {
			return "", errors.New("the port is gone")
		}}

		_, _, err := charAt(context.Background(), device.New(conn), 0)
		if err == nil {
			t.Fatal("expected an error when the screen cannot be read, got none")
		}
		if !strings.Contains(err.Error(), "reading the text entry screen") {
			t.Errorf("expected the message to say it was reading the screen, got: %v", err)
		}
	})
}

// Test_check tests the check function with 100% coverage.
//
// Coverage: 100% (4 test cases covering all branches)
//
// Test cases:
//   - Accepted: a value the screen can hold passes
//   - NoLimit: a screen reporting no limit accepts any length
//   - TooLong: a value longer than the screen holds is refused
//   - Unaccepted: a character the screen does not accept is refused
func Test_check(t *testing.T) {
	// Verify that a value the screen can hold passes
	t.Run("Accepted", func(t *testing.T) {
		if err := check("AB", " ABC", 8); err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
	})

	// Verify that a screen reporting no limit accepts a value of any length
	t.Run("NoLimit", func(t *testing.T) {
		if err := check("ABCABC", " ABC", 0); err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
	})

	// Verify that a value longer than the screen holds is refused
	t.Run("TooLong", func(t *testing.T) {
		err := check("ABCD", " ABC", 2)
		if err == nil {
			t.Fatal("expected an error for a value that is too long, got none")
		}
		if !strings.Contains(err.Error(), "holds at most 2") {
			t.Errorf("expected the message to give the limit, got: %v", err)
		}
	})

	// Verify that a character the screen does not accept is refused
	t.Run("Unaccepted", func(t *testing.T) {
		err := check("AZ", " ABC", 8)
		if err == nil {
			t.Fatal("expected an error for an unaccepted character, got none")
		}
		if !strings.Contains(err.Error(), "character 2") {
			t.Errorf("expected the message to say which character, got: %v", err)
		}
	})
}

// Test_cycleOrder tests the cycleOrder function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - NotReported: an unreported character set falls back to the measured one
//   - TrailingSpace: the space the scanner lists last is moved to the front
//   - NoTrailingSpace: a set without a trailing space is used as reported
func Test_cycleOrder(t *testing.T) {
	// Verify that an unreported character set falls back to the measured one
	t.Run("NotReported", func(t *testing.T) {
		if got := cycleOrder(""); got != fallbackOrder {
			t.Errorf("expected the fallback order, got %q", got)
		}
	})

	// Verify that the space the scanner lists last is moved to the front
	t.Run("TrailingSpace", func(t *testing.T) {
		if got := cycleOrder("ABC "); got != " ABC" {
			t.Errorf("expected \" ABC\", got %q", got)
		}
	})

	// Verify that a set without a trailing space is used exactly as reported
	t.Run("NoTrailingSpace", func(t *testing.T) {
		if got := cycleOrder("0123456789."); got != "0123456789." {
			t.Errorf("expected the set unchanged, got %q", got)
		}
	})
}

// Test_press tests the press function with 100% coverage.
//
// Coverage: 100% (2 test cases covering all branches)
//
// Test cases:
//   - Success: the key reaches the scanner
//   - Refused: a key the scanner does not take is reported
func Test_press(t *testing.T) {
	// Verify that the key reaches the scanner
	t.Run("Success", func(t *testing.T) {
		var sent string
		conn := &stubConn{exec: func(command string) (string, error) {
			sent = command
			return "", nil
		}}

		if err := press(context.Background(), device.New(conn), commit); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if sent != "KEY,E,P" {
			t.Errorf("expected the accept key to be sent, got %q", sent)
		}
	})

	// Verify that a key the scanner does not take is reported
	t.Run("Refused", func(t *testing.T) {
		conn := &stubConn{exec: func(command string) (string, error) {
			return "", errors.New("the scanner refused the key")
		}}

		err := press(context.Background(), device.New(conn), commit)
		if err == nil {
			t.Fatal("expected an error when the key is refused, got none")
		}
		if !strings.Contains(err.Error(), "pressing a key on the text entry screen") {
			t.Errorf("expected the message to say it was pressing a key, got: %v", err)
		}
	})
}

// Test_route tests the route function with 100% coverage.
//
// Coverage: 100% (5 test cases covering all branches)
//
// Test cases:
//   - Forward: the short way round is forward
//   - Back: the short way round is backward
//   - Unaccepted: a character outside the set cannot be reached
//   - StartOutsideSet: an unexpected current character is counted from the space
//   - NoSpaceToCountFrom: neither the character nor a space is in the set
func Test_route(t *testing.T) {
	// Verify that the knob turns forward when that is the short way round
	t.Run("Forward", func(t *testing.T) {
		steps, key, err := route(' ', 'B', " ABCDE")
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if steps != 2 || key != cycleForward {
			t.Errorf("expected 2 turns forward, got %d of %q", steps, string(key))
		}
	})

	// Verify that the knob turns back when that is the short way round
	t.Run("Back", func(t *testing.T) {
		steps, key, err := route(' ', 'E', " ABCDE")
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if steps != 1 || key != cycleBack {
			t.Errorf("expected 1 turn back, got %d of %q", steps, string(key))
		}
	})

	// Verify that a character the screen does not accept cannot be reached
	t.Run("Unaccepted", func(t *testing.T) {
		_, _, err := route('A', 'Z', " ABC")
		if err == nil {
			t.Fatal("expected an error for an unaccepted character, got none")
		}
		if !strings.Contains(err.Error(), "does not accept") {
			t.Errorf("expected the message to say the character is not accepted, got: %v", err)
		}
	})

	// Verify that a current character outside the set is counted from the space
	t.Run("StartOutsideSet", func(t *testing.T) {
		steps, key, err := route('Z', 'B', " ABC")
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if steps != 2 || key != cycleForward {
			t.Errorf("expected 2 turns forward from the space, got %d of %q", steps, string(key))
		}
	})

	// Verify that a set holding neither the character nor a space is refused
	t.Run("NoSpaceToCountFrom", func(t *testing.T) {
		_, _, err := route('Z', 'B', "ABC")
		if err == nil {
			t.Fatal("expected an error when there is no space to count from, got none")
		}
		if !strings.Contains(err.Error(), "cannot work out how to reach") {
			t.Errorf("expected the message to say it cannot work out a route, got: %v", err)
		}
	})
}

// Test_setChar tests the setChar function with 100% coverage.
//
// Coverage: 100% (13 test cases covering all branches)
//
// Test cases:
//   - AlreadyCorrect: a position already holding the character is left alone
//   - ReadError: a screen that cannot be read is reported
//   - Blanks: a space is set with one press rather than by cycling
//   - BlanksPastEnd: a space that only looks right past the end is still set
//   - BlankPressError: a refused blanking press is reported
//   - PastEnd: a position past the end is nudged into a known state first
//   - GenuineSpace: a real space is nudged too, not routed away from
//   - NudgePressError: a refused nudging press is reported
//   - Turns: a character is turned to and confirmed
//   - RouteError: a character that cannot be reached names its position
//   - TurnPressError: a refused turning press is reported
//   - GivesUp: a position that never takes the character is reported
//   - GivesUpReadError: a failed final read is reported
func Test_setChar(t *testing.T) {
	// Builds the document the scanner answers a menu read with.
	menuDoc := func(value string) string {
		return fmt.Sprintf(`<MenuInfo Name="Name" MenuType="TypeInput" Value="%s"><MenuInput MaxLength="8" EnableKeys="ABC "/></MenuInfo>`, value)
	}

	// Verify that a position already holding the character is left alone
	t.Run("AlreadyCorrect", func(t *testing.T) {
		pressed := false
		conn := &stubConn{
			execXML: func(command string) (string, error) { return menuDoc("AB"), nil },
			exec: func(command string) (string, error) {
				pressed = true
				return "", nil
			},
		}

		if err := setChar(context.Background(), device.New(conn), 0, 'A', " ABC"); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if pressed {
			t.Error("expected nothing to be pressed when the character is already right")
		}
	})

	// Verify that a screen that cannot be read is reported
	t.Run("ReadError", func(t *testing.T) {
		conn := &stubConn{execXML: func(command string) (string, error) {
			return "", errors.New("the port is gone")
		}}

		if err := setChar(context.Background(), device.New(conn), 0, 'A', " ABC"); err == nil {
			t.Error("expected an error when the screen cannot be read, got none")
		}
	})

	// Verify that a space is set with one press rather than by cycling round to it
	t.Run("Blanks", func(t *testing.T) {
		value := "AB"
		var keys []string
		conn := &stubConn{
			execXML: func(command string) (string, error) { return menuDoc(value), nil },
			exec: func(command string) (string, error) {
				keys = append(keys, command)
				if command == "KEY,.,P" {
					value = " B"
				}
				return "", nil
			},
		}

		if err := setChar(context.Background(), device.New(conn), 0, ' ', " ABC"); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if len(keys) != 1 || keys[0] != "KEY,.,P" {
			t.Errorf("expected one blanking press, got %v", keys)
		}
	})

	// Verify that a space past the end of the value is set rather than trusted
	t.Run("BlanksPastEnd", func(t *testing.T) {
		value := ""
		var keys []string
		conn := &stubConn{
			execXML: func(command string) (string, error) { return menuDoc(value), nil },
			exec: func(command string) (string, error) {
				keys = append(keys, command)

				// Blanking the cell makes the value reach that far, so the
				// space it now reports is a reading rather than a guess.
				if command == "KEY,.,P" {
					value = " "
				}
				return "", nil
			},
		}

		// The position already looks like a space, but the value does not reach
		// it, so the space cannot be trusted and is set anyway.
		if err := setChar(context.Background(), device.New(conn), 0, ' ', " ABC"); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if len(keys) != 1 || keys[0] != "KEY,.,P" {
			t.Errorf("expected one blanking press, got %v", keys)
		}
	})

	// Verify that a refused blanking press is reported
	t.Run("BlankPressError", func(t *testing.T) {
		conn := &stubConn{
			execXML: func(command string) (string, error) { return menuDoc("AB"), nil },
			exec: func(command string) (string, error) {
				return "", errors.New("the scanner refused the key")
			},
		}

		if err := setChar(context.Background(), device.New(conn), 0, ' ', " ABC"); err == nil {
			t.Error("expected an error when the blanking press is refused, got none")
		}
	})

	// Verify that a position past the end of the value is nudged into a known state
	t.Run("PastEnd", func(t *testing.T) {
		value := ""
		conn := &stubConn{
			execXML: func(command string) (string, error) { return menuDoc(value), nil },
			exec: func(command string) (string, error) {
				// The nudge puts something real in the cell.
				if command == "KEY,>,P" {
					value = "A"
				}
				return "", nil
			},
		}

		if err := setChar(context.Background(), device.New(conn), 0, 'A', " ABC"); err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
	})

	// Verify that a real space is nudged as well, rather than routed away from
	t.Run("GenuineSpace", func(t *testing.T) {
		value := " B"
		conn := &stubConn{
			execXML: func(command string) (string, error) { return menuDoc(value), nil },
			exec: func(command string) (string, error) {
				// The knob does not move away from a space by the number of
				// places the character set implies, so one press settles it.
				if command == "KEY,>,P" {
					value = "AB"
				}
				return "", nil
			},
		}

		if err := setChar(context.Background(), device.New(conn), 0, 'A', " ABC"); err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
	})

	// Verify that a refused nudging press is reported
	t.Run("NudgePressError", func(t *testing.T) {
		conn := &stubConn{
			execXML: func(command string) (string, error) { return menuDoc(""), nil },
			exec: func(command string) (string, error) {
				return "", errors.New("the scanner refused the key")
			},
		}

		if err := setChar(context.Background(), device.New(conn), 0, 'A', " ABC"); err == nil {
			t.Error("expected an error when the nudging press is refused, got none")
		}
	})

	// Verify that a character is turned to and confirmed by reading it back
	t.Run("Turns", func(t *testing.T) {
		value := "A"
		turns := 0
		conn := &stubConn{
			execXML: func(command string) (string, error) { return menuDoc(value), nil },
			exec: func(command string) (string, error) {
				if command == "KEY,>,P" {
					turns++
					value = "C"
				}
				return "", nil
			},
		}

		if err := setChar(context.Background(), device.New(conn), 0, 'C', " ABC"); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		// "A" is at index 1 and "C" at index 3 of " ABC", which is two turns.
		if turns != 2 {
			t.Errorf("expected 2 turns of the knob, got %d", turns)
		}
	})

	// Verify that a character that cannot be reached names the position it failed at
	t.Run("RouteError", func(t *testing.T) {
		conn := &stubConn{execXML: func(command string) (string, error) {
			return menuDoc("AB"), nil
		}}

		err := setChar(context.Background(), device.New(conn), 1, 'Z', " ABC")
		if err == nil {
			t.Fatal("expected an error for an unreachable character, got none")
		}
		if !strings.Contains(err.Error(), "at position 2") {
			t.Errorf("expected the message to name the position, got: %v", err)
		}
	})

	// Verify that a refused turning press is reported
	t.Run("TurnPressError", func(t *testing.T) {
		conn := &stubConn{
			execXML: func(command string) (string, error) { return menuDoc("A"), nil },
			exec: func(command string) (string, error) {
				return "", errors.New("the scanner refused the key")
			},
		}

		if err := setChar(context.Background(), device.New(conn), 0, 'C', " ABC"); err == nil {
			t.Error("expected an error when the turning press is refused, got none")
		}
	})

	// Verify that a position that never takes the character is reported
	t.Run("GivesUp", func(t *testing.T) {
		conn := &stubConn{
			execXML: func(command string) (string, error) { return menuDoc("A"), nil },
			exec:    func(command string) (string, error) { return "", nil },
		}

		err := setChar(context.Background(), device.New(conn), 0, 'C', " ABC")
		if err == nil {
			t.Fatal("expected an error when the character never takes, got none")
		}
		if !strings.Contains(err.Error(), "after 5 attempts") {
			t.Errorf("expected the message to give up after every attempt, got: %v", err)
		}
	})

	// Verify that a failed read after the last attempt is reported
	t.Run("GivesUpReadError", func(t *testing.T) {
		reads := 0
		conn := &stubConn{
			execXML: func(command string) (string, error) {
				reads++

				// The final read, the one that reports what it ended up
				// holding, is the one that fails.
				if reads > rounds {
					return "", errors.New("the port is gone")
				}
				return menuDoc("A"), nil
			},
			exec: func(command string) (string, error) { return "", nil },
		}

		err := setChar(context.Background(), device.New(conn), 0, 'C', " ABC")
		if err == nil {
			t.Fatal("expected an error when the final read fails, got none")
		}
		if !strings.Contains(err.Error(), "reading the text entry screen") {
			t.Errorf("expected the message to say it was reading the screen, got: %v", err)
		}
	})
}

// Test_typeDirect tests the typeDirect function with 100% coverage.
//
// Coverage: 100% (7 test cases covering all branches)
//
// Test cases:
//   - AlreadyCorrect: a value that is already right is accepted without typing
//   - AlreadyHoldsAnother: a screen holding something else is refused
//   - UnsendableCharacter: a character with no known key is refused
//   - PressError: a refused key press is reported
//   - ReadBackError: a screen that cannot be read back is reported
//   - Mismatch: a press the scanner missed is caught
//   - Success: the value is typed and accepted
func Test_typeDirect(t *testing.T) {
	// Builds the document the scanner answers a menu read with.
	menuDoc := func(value string) string {
		return fmt.Sprintf(`<MenuInfo Name="Name" MenuType="TypeInput" Value="%s"><MenuInput MaxLength="8" EnableKeys="0123456789."/></MenuInfo>`, value)
	}

	// Verify that a value that is already right is accepted without typing
	t.Run("AlreadyCorrect", func(t *testing.T) {
		var keys []string
		conn := &stubConn{exec: func(command string) (string, error) {
			keys = append(keys, command)
			return "", nil
		}}

		if err := typeDirect(context.Background(), device.New(conn), "123", "123"); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if len(keys) != 1 || keys[0] != "KEY,E,P" {
			t.Errorf("expected only the accept key to be pressed, got %v", keys)
		}
	})

	// Verify that a screen already holding something else is refused
	t.Run("AlreadyHoldsAnother", func(t *testing.T) {
		conn := &stubConn{}

		err := typeDirect(context.Background(), device.New(conn), "456", "123")
		if err == nil {
			t.Fatal("expected an error when the screen holds another value, got none")
		}
		if !strings.Contains(err.Error(), "no known way to clear it") {
			t.Errorf("expected the message to say it cannot be cleared, got: %v", err)
		}
	})

	// Verify that a character with no known key is refused before anything is pressed
	t.Run("UnsendableCharacter", func(t *testing.T) {
		pressed := false
		conn := &stubConn{exec: func(command string) (string, error) {
			pressed = true
			return "", nil
		}}

		err := typeDirect(context.Background(), device.New(conn), "", "12-3")
		if err == nil {
			t.Fatal("expected an error for a character with no known key, got none")
		}
		if !strings.Contains(err.Error(), "cannot be entered from here") {
			t.Errorf("expected the message to say it cannot be entered, got: %v", err)
		}
		if pressed {
			t.Error("expected nothing to be pressed when the value is refused")
		}
	})

	// Verify that a refused key press is reported
	t.Run("PressError", func(t *testing.T) {
		conn := &stubConn{exec: func(command string) (string, error) {
			return "", errors.New("the scanner refused the key")
		}}

		if err := typeDirect(context.Background(), device.New(conn), "", "12"); err == nil {
			t.Error("expected an error when a press is refused, got none")
		}
	})

	// Verify that a screen that cannot be read back is reported
	t.Run("ReadBackError", func(t *testing.T) {
		conn := &stubConn{
			execXML: func(command string) (string, error) {
				return "", errors.New("the port is gone")
			},
			exec: func(command string) (string, error) { return "", nil },
		}

		err := typeDirect(context.Background(), device.New(conn), "", "12")
		if err == nil {
			t.Fatal("expected an error when the screen cannot be read back, got none")
		}
		if !strings.Contains(err.Error(), "reading the entry screen back") {
			t.Errorf("expected the message to say it was reading the screen back, got: %v", err)
		}
	})

	// Verify that a press the scanner missed is caught by reading the value back
	t.Run("Mismatch", func(t *testing.T) {
		conn := &stubConn{
			execXML: func(command string) (string, error) { return menuDoc("1"), nil },
			exec:    func(command string) (string, error) { return "", nil },
		}

		err := typeDirect(context.Background(), device.New(conn), "", "12")
		if err == nil {
			t.Fatal("expected an error when a press was missed, got none")
		}
		if !strings.Contains(err.Error(), "did not take one of the presses") {
			t.Errorf("expected the message to say a press was missed, got: %v", err)
		}
	})

	// Verify that the value is typed one key per character and then accepted
	t.Run("Success", func(t *testing.T) {
		var keys []string
		conn := &stubConn{
			execXML: func(command string) (string, error) { return menuDoc("12"), nil },
			exec: func(command string) (string, error) {
				keys = append(keys, command)
				return "", nil
			},
		}

		if err := typeDirect(context.Background(), device.New(conn), "", "12"); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		// One key per character, then the accept key.
		want := []string{"KEY,1,P", "KEY,2,P", "KEY,E,P"}
		if len(keys) != len(want) {
			t.Fatalf("expected %d presses, got %v", len(want), keys)
		}
		for i := range want {
			if keys[i] != want[i] {
				t.Errorf("expected press %d to be %q, got %q", i, want[i], keys[i])
			}
		}
	})
}

// Test_typed tests the typed function with 100% coverage.
//
// Coverage: 100% (3 test cases covering all branches)
//
// Test cases:
//   - NumberScreen: a set of only digits is a screen the keypad can type on
//   - NameScreen: a set holding letters is turned to with the knob
//   - Empty: a screen accepting nothing is not a number screen
func Test_typed(t *testing.T) {
	// Verify that a set of only digits is a screen the keypad can type on
	t.Run("NumberScreen", func(t *testing.T) {
		if !typed("0123456789.") {
			t.Error("expected a set of only digits to be typed on the keypad")
		}
	})

	// Verify that a set holding letters is a name screen, turned to with the knob
	t.Run("NameScreen", func(t *testing.T) {
		if typed(" ABC") {
			t.Error("expected a set holding letters not to be typed on the keypad")
		}
	})

	// Verify that a screen accepting nothing is not treated as a number screen
	t.Run("Empty", func(t *testing.T) {
		if typed("") {
			t.Error("expected an empty set not to be typed on the keypad")
		}
	})
}
