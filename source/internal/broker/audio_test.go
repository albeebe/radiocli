// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/11/2026

package broker

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/albeebe/radiocli/internal/appcontext"
	"github.com/albeebe/radiocli/internal/audiofeed"
)

func TestFrameRoundTrip(t *testing.T) {
	cases := []struct {
		name    string
		kind    byte
		payload []byte
	}{
		{"an empty payload", FrameJSON, []byte{}},
		{"one byte", FrameAudio, []byte{0x7f}},
		{"an audio frame the size of raw samples", FrameAudio, bytes.Repeat([]byte{0xab}, 4+1920)},
		{"a payload that is not text", FrameAudio, []byte{0x00, 0xff, 0xfe, 0x80, 0x01}},
		{"the largest allowed", FrameAudio, bytes.Repeat([]byte{0x5a}, MaxFrame)},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := writeFrame(&buf, c.kind, c.payload); err != nil {
				t.Fatalf("writeFrame: %v", err)
			}

			kind, got, err := readFrame(bufio.NewReader(&buf))
			if err != nil {
				t.Fatalf("readFrame: %v", err)
			}
			if kind != c.kind {
				t.Errorf("kind came back as %d, want %d", kind, c.kind)
			}
			if !bytes.Equal(got, c.payload) {
				t.Errorf("the payload came back as %d bytes, want %d", len(got), len(c.payload))
			}
		})
	}
}

// TestFrameRoundTripBackToBack is the property that matters most about the
// framing: several frames on one stream have to come apart again in the same
// places. Nothing in the bytes says where a frame begins, so a length written
// or read wrongly loses the rest of the connection rather than one frame.
func TestFrameRoundTripBackToBack(t *testing.T) {
	var buf bytes.Buffer

	sizes := []int{0, 1, 81, 1924, 7, 1279}
	for i, size := range sizes {
		if err := writeFrame(&buf, FrameAudio, bytes.Repeat([]byte{byte(i)}, size)); err != nil {
			t.Fatal(err)
		}
	}

	in := bufio.NewReader(&buf)
	for i, size := range sizes {
		kind, got, err := readFrame(in)
		if err != nil {
			t.Fatalf("frame %d: %v", i, err)
		}
		if kind != FrameAudio || len(got) != size {
			t.Fatalf("frame %d came back as kind %d and %d bytes, want %d and %d",
				i, kind, len(got), FrameAudio, size)
		}
		for _, b := range got {
			if b != byte(i) {
				t.Fatalf("frame %d holds bytes from another frame", i)
			}
		}
	}

	if _, _, err := readFrame(in); err != io.EOF {
		t.Errorf("after the last frame the reader gave %v, want EOF", err)
	}
}

func TestWriteFrameRefusesAnOversizedPayload(t *testing.T) {
	var buf bytes.Buffer
	if err := writeFrame(&buf, FrameAudio, make([]byte, MaxFrame+1)); err == nil {
		t.Error("writeFrame accepted a payload over the limit")
	}
	if buf.Len() != 0 {
		t.Error("writeFrame wrote part of a frame it then refused")
	}
}

// TestReadFrameRefusesAnOversizedLength stops a length read out of a corrupt or
// hostile stream turning into an allocation of whatever it says.
func TestReadFrameRefusesAnOversizedLength(t *testing.T) {
	// A header claiming 16 MiB, which is the largest 24 bits can say.
	head := []byte{FrameAudio, 0xff, 0xff, 0xff}
	_, _, err := readFrame(bufio.NewReader(bytes.NewReader(head)))
	if err == nil {
		t.Fatal("readFrame accepted a length over the limit")
	}
	if !strings.Contains(err.Error(), "limit") {
		t.Errorf("the refusal reads %q, which does not say why", err)
	}
}

func TestReadFrameRefusesATruncatedStream(t *testing.T) {
	var buf bytes.Buffer
	writeFrame(&buf, FrameAudio, bytes.Repeat([]byte{1}, 100))
	whole := buf.Bytes()

	for _, at := range []int{1, 3, 4, 50, len(whole) - 1} {
		_, _, err := readFrame(bufio.NewReader(bytes.NewReader(whole[:at])))
		if err == nil {
			t.Errorf("readFrame accepted a stream cut off at %d bytes", at)
		}
	}
}

// fakeCapture stands in for a sound card, so everything above it can be tested
// on a machine with none and in a test that must not raise a permission prompt.
type fakeCapture struct {
	source, channel string
	closed          bool
}

func (f *fakeCapture) Source() string  { return f.source }
func (f *fakeCapture) Channel() string { return f.channel }
func (f *fakeCapture) Close()          { f.closed = true }

// audioDaemon is a daemon holding a sound card that is not there, plus a handle
// on the feed so a test can decide what it hears.
type audioDaemon struct {
	srv  *Server
	ctx  context.Context
	stop context.CancelFunc

	mu      sync.Mutex
	out     audiofeed.Publisher
	capture *fakeCapture

	conns sync.WaitGroup
}

func newAudioDaemon(t *testing.T) *audioDaemon {
	t.Helper()

	app := appcontext.New()
	app.Config = appcontext.Defaults()
	app.Log = slog.New(slog.DiscardHandler)
	app.Stdout, app.Stderr = io.Discard, io.Discard

	ctx, stop := context.WithCancel(context.Background())
	d := &audioDaemon{ctx: ctx, stop: stop}

	side := newAudioSide("Test Line In", audiofeed.ChannelLeft, app.Log)
	side.open = func(opts audiofeed.Options, out audiofeed.Publisher) (audioCapture, error) {
		d.mu.Lock()
		defer d.mu.Unlock()
		d.out = out
		d.capture = &fakeCapture{source: opts.Source, channel: opts.Channel}
		return d.capture, nil
	}

	d.srv = &Server{app: app, run: runner{app: app}, log: app.Log, audio: side}
	t.Cleanup(func() {
		stop()
		d.conns.Wait()
	})
	return d
}

// connect opens a connection to this daemon and returns the client end.
//
// A real unix socket rather than net.Pipe, and that is the whole point of this
// harness. net.Pipe matches each read to one write and never runs them
// together, so a client that buffered ahead past the end of a line would look
// perfectly correct on it. A kernel socket does run them together: the reply
// and the frames behind it arrive in one read, which is exactly the case where
// reading ahead loses audio. Measured on this machine, a client using a
// bufio.Scanner for the handshake swallows the first three frames.
func (d *audioDaemon) connect(t *testing.T) net.Conn {
	t.Helper()

	// Short, because a unix socket path is limited to about a hundred bytes on
	// macOS and the usual temporary directory is most of that already.
	dir, err := os.MkdirTemp("", "b")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	path := filepath.Join(dir, "s")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })

	d.conns.Add(1)
	go func() {
		defer d.conns.Done()
		server, err := listener.Accept()
		if err != nil {
			return
		}
		d.srv.serveConn(d.ctx, server)
	}()

	client, err := net.Dial("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })

	// See socketPair: a lost frame should stop the test rather than hang it.
	client.SetReadDeadline(time.Now().Add(10 * time.Second))
	return client
}

// publish hands one frame to whatever is listening, waiting for the capture to
// have been opened first.
//
// It reports rather than failing outright, because several tests call it from a
// goroutine and only the test's own goroutine may stop a test. A frame that was
// never published shows up anyway, as the frame that never arrived.
func (d *audioDaemon) publish(t *testing.T, frame audiofeed.Frame) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for {
		d.mu.Lock()
		out := d.out
		d.mu.Unlock()

		if out != nil {
			out.Publish(frame)
			return
		}
		if time.Now().After(deadline) {
			t.Error("the daemon never opened its capture")
			return
		}
		time.Sleep(time.Millisecond)
	}
}

func pcmFrame(seq uint32, v byte) audiofeed.Frame {
	return audiofeed.Frame{
		Seq: seq,
		PCM: bytes.Repeat([]byte{v}, audiofeed.MonoFrameBytes),
		At:  time.Now(),
	}
}

// TestAudioStreamHandsOverFromLinesToFrames is the most important test here.
//
// An audio connection begins as newline JSON and becomes frames. The obvious
// way to read it, a bufio.Scanner for the lines and then the socket for the
// frames, silently loses whatever the Scanner had already buffered, which on a
// fast daemon is the first several frames. It looks like a codec fault and it is
// not one. This drives the real client against the real daemon and checks that
// the very first frame arrives whole.
func TestAudioStreamHandsOverFromLinesToFrames(t *testing.T) {
	d := newAudioDaemon(t)
	client := d.connect(t)

	// Opened on a goroutine because the daemon answers on the same pipe, and a
	// pipe has no buffer of its own.
	type opened struct {
		stream *AudioStream
		err    error
	}
	got := make(chan opened, 1)
	go func() {
		s, err := openAudio(client, FormatPCM, 0)
		got <- opened{s, err}
	}()

	var stream *AudioStream
	select {
	case r := <-got:
		if r.err != nil {
			t.Fatalf("opening the stream: %v", r.err)
		}
		stream = r.stream
	case <-time.After(5 * time.Second):
		t.Fatal("the stream never opened")
	}

	if info := stream.Info(); info.Format != FormatPCM || info.Rate != audiofeed.SampleRate {
		t.Errorf("the stream says %q at %d Hz, want %q at %d",
			info.Format, info.Rate, FormatPCM, audiofeed.SampleRate)
	}
	if got := stream.Info().Source; got != "Test Line In" {
		t.Errorf("the stream names its source %q, want %q", got, "Test Line In")
	}

	// Several frames in a row, sent as fast as they can be, which is what fills
	// a read-ahead buffer if there is one.
	const frames = 8
	go func() {
		for i := range uint32(frames) {
			d.publish(t, pcmFrame(i, byte(i+1)))
		}
	}()

	for i := range uint32(frames) {
		seq, audio, event, err := stream.Next()
		if err != nil {
			t.Fatalf("frame %d: %v", i, err)
		}
		if event != nil {
			t.Fatalf("frame %d came back as an event, not audio", i)
		}
		if seq != i {
			t.Fatalf("frame %d is numbered %d: the handover lost frames", i, seq)
		}
		if len(audio) != audiofeed.MonoFrameBytes {
			t.Fatalf("frame %d is %d bytes, want %d", i, len(audio), audiofeed.MonoFrameBytes)
		}
		for _, b := range audio {
			if b != byte(i+1) {
				t.Fatalf("frame %d holds the wrong audio", i)
			}
		}
	}
}

// TestAudioStreamSurvivesTheReplyAndFramesArrivingTogether is the deterministic
// half of the handover check, and the one that would actually fail if the client
// read ahead.
//
// The integration test above depends on the daemon getting frames out before the
// client reads the reply, which is a race it usually wins but not a thing to
// rely on. This forces the case: the reply and eight frames go out in a single
// write, so they are certain to arrive in one read.
//
// Measured against a client that used a bufio.Scanner for the reply and then
// read frames from the socket, this loses the first three frames, and it loses
// them silently. That is the bug this test exists for.
func TestAudioStreamSurvivesTheReplyAndFramesArrivingTogether(t *testing.T) {
	client, server := socketPair(t)

	const frames = 8
	go func() {
		// The request, which this synthetic daemon does not bother reading.
		one := make([]byte, 0, 1024)
		one = append(one, []byte(`{"type":"hello","protocol":3}`+"\n")...)
		server.Write(one)

		// Everything else in one write: the reply and every frame behind it.
		var buf bytes.Buffer
		buf.WriteString(`{"type":"audio","format":"pcm","rate":48000,"channels":1,"frameMs":20}` + "\n")
		for i := range frames {
			payload := make([]byte, 4+8)
			payload[3] = byte(i)
			for at := 4; at < len(payload); at++ {
				payload[at] = byte(i + 1)
			}
			writeFrame(&buf, FrameAudio, payload)
		}
		server.Write(buf.Bytes())
	}()

	stream, err := openAudio(client, FormatPCM, 0)
	if err != nil {
		t.Fatalf("opening the stream: %v", err)
	}

	for i := range uint32(frames) {
		seq, audio, _, err := stream.Next()
		if err != nil {
			t.Fatalf("frame %d: %v", i, err)
		}
		if seq != i {
			t.Fatalf("the first frames after the reply are numbered from %d, want 0: "+
				"the handshake read past the end of the line and ate them", seq)
		}
		for _, b := range audio {
			if b != byte(i+1) {
				t.Fatalf("frame %d holds the wrong audio", i)
			}
		}
	}
}

// socketPair returns two ends of a real unix socket. See connect for why this
// is not net.Pipe.
func socketPair(t *testing.T) (client, server net.Conn) {
	t.Helper()

	dir, err := os.MkdirTemp("", "b")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	listener, err := net.Listen("unix", filepath.Join(dir, "s"))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	accepted := make(chan net.Conn, 1)
	go func() {
		c, err := listener.Accept()
		if err != nil {
			close(accepted)
			return
		}
		accepted <- c
	}()

	client, err = net.Dial("unix", filepath.Join(dir, "s"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })

	// A deadline so that a client which lost frames fails rather than waits for
	// ones that are never coming. That is exactly how the read-ahead bug
	// presents, and a test that hangs says far less than one that stops.
	client.SetReadDeadline(time.Now().Add(10 * time.Second))

	server, ok := <-accepted
	if !ok {
		t.Fatal("nothing accepted the connection")
	}
	t.Cleanup(func() { server.Close() })
	return client, server
}

// TestAudioStreamCarriesOpus checks the compressed tier end to end, including
// that packets are far smaller than the samples they came from.
func TestAudioStreamCarriesOpus(t *testing.T) {
	d := newAudioDaemon(t)
	client := d.connect(t)

	got := make(chan *AudioStream, 1)
	go func() {
		s, err := openAudio(client, FormatOpus, 32000)
		if err != nil {
			t.Error(err)
			close(got)
			return
		}
		got <- s
	}()

	stream, ok := <-got
	if !ok {
		t.Fatal("the stream never opened")
	}
	if info := stream.Info(); info.Format != FormatOpus || info.Bitrate != 32000 {
		t.Fatalf("the stream says %q at %d, want %q at 32000", info.Format, info.Bitrate, FormatOpus)
	}

	go func() {
		for i := range uint32(4) {
			d.publish(t, pcmFrame(i, 0))
		}
	}()

	for i := range uint32(4) {
		seq, audio, _, err := stream.Next()
		if err != nil {
			t.Fatalf("packet %d: %v", i, err)
		}
		if seq != i {
			t.Errorf("packet %d is numbered %d", i, seq)
		}
		if len(audio) == 0 || len(audio) >= audiofeed.MonoFrameBytes {
			t.Errorf("packet %d is %d bytes, which is not compressed", i, len(audio))
		}
	}
}

// TestAudioConnectionRefusesAnotherOperation: a connection carrying audio has
// stopped speaking newline JSON in this direction, so there is nowhere for the
// answer to anything else to go. What must not happen is a bare line landing in
// the middle of the frames.
func TestAudioConnectionSendsErrorsAsFrames(t *testing.T) {
	d := newAudioDaemon(t)
	client := d.connect(t)

	got := make(chan *AudioStream, 1)
	go func() {
		s, err := openAudio(client, FormatPCM, 0)
		if err != nil {
			t.Error(err)
			close(got)
			return
		}
		got <- s
	}()

	stream, ok := <-got
	if !ok {
		t.Fatal("the stream never opened")
	}

	// Something the daemon cannot make sense of at all. This is the path that
	// does not know what kind of connection it is on, which is exactly why the
	// decision lives in send rather than at each call site.
	if _, err := client.Write([]byte("this is not json\n")); err != nil {
		t.Fatal(err)
	}

	_, audio, event, err := stream.Next()
	if err != nil {
		t.Fatalf("reading after a bad request: %v", err)
	}
	if audio != nil {
		t.Fatal("the daemon answered a malformed request with audio")
	}
	if event == nil {
		t.Fatal("the refusal did not arrive as a framed message")
	}

	var msg Response
	if err := json.Unmarshal(event, &msg); err != nil {
		t.Fatalf("the framed refusal is not JSON: %v", err)
	}
	if msg.Type != TypeError {
		t.Errorf("the refusal came back as %q, want %q", msg.Type, TypeError)
	}

	// And the stream carries on afterwards, which it could not do if the line
	// had gone out unframed: nothing in the bytes says where a frame begins, so
	// one stray line would desynchronise the rest of the connection.
	go d.publish(t, pcmFrame(99, 7))

	seq, audio, _, err := stream.Next()
	if err != nil {
		t.Fatalf("the stream did not survive the refusal: %v", err)
	}
	if seq != 99 || len(audio) != audiofeed.MonoFrameBytes {
		t.Errorf("after the refusal a frame came back as %d and %d bytes, want 99 and %d",
			seq, len(audio), audiofeed.MonoFrameBytes)
	}
}

// TestAudioConnectionClosesCleanly is the deadlock the deferred order in
// serveConn is arranged to avoid. A stream ends when its write fails, and its
// write fails when the socket closes, which is the last thing serveConn does.
// Waiting for the stream before closing the socket would be waiting forever.
func TestAudioConnectionClosesCleanly(t *testing.T) {
	d := newAudioDaemon(t)
	client, server := net.Pipe()

	done := make(chan struct{})
	go func() {
		defer close(done)
		d.srv.serveConn(d.ctx, server)
	}()

	go func() {
		s, err := openAudio(client, FormatPCM, 0)
		if err != nil {
			return
		}
		// Read one frame so the stream is genuinely running, then stop reading
		// entirely, which is what a listener that was closed does.
		go d.publish(t, pcmFrame(0, 1))
		s.Next()
	}()

	// Give the stream time to start before pulling the socket away.
	time.Sleep(200 * time.Millisecond)
	client.Close()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the connection never finished tearing down after the client went away")
	}
}

// TestTwoClientsShareOneSoundCard is the whole reason the daemon holds the
// sound card, tested through the real protocol rather than at the fan-out
// underneath it.
//
// A sound card can only be open once, so without this two things wanting the
// scanner's audio means one of them gets it and the other gets an error. Here
// they both connect, both ask, and both hear the same audio, and the card is
// opened once for the pair of them.
//
// It also covers the tiers being independent: one takes raw samples and the
// other takes Opus, off one capture, which is what lets a listener on a phone and
// a program on the same machine be served at once.
func TestTwoClientsShareOneSoundCard(t *testing.T) {
	d := newAudioDaemon(t)

	open := func(format string) *AudioStream {
		t.Helper()
		client := d.connect(t)

		got := make(chan *AudioStream, 1)
		go func() {
			s, err := openAudio(client, format, 32000)
			if err != nil {
				t.Error(err)
				close(got)
				return
			}
			got <- s
		}()

		select {
		case s, ok := <-got:
			if !ok {
				t.Fatal("the stream never opened")
			}
			return s
		case <-time.After(5 * time.Second):
			t.Fatal("the stream never opened")
			return nil
		}
	}

	raw := open(FormatPCM)
	compressed := open(FormatOpus)

	d.mu.Lock()
	opened := d.capture
	d.mu.Unlock()
	if opened == nil {
		t.Fatal("no capture was opened")
	}

	const frames = 6
	go func() {
		for i := range uint32(frames) {
			d.publish(t, pcmFrame(i, byte(i+1)))
		}
	}()

	for i := range uint32(frames) {
		seq, audio, event, err := raw.Next()
		if err != nil {
			t.Fatalf("the raw listener failed on frame %d: %v", i, err)
		}
		if event != nil {
			continue
		}
		if seq != i {
			t.Fatalf("the raw listener got frame %d, want %d", seq, i)
		}
		if len(audio) != audiofeed.MonoFrameBytes {
			t.Fatalf("the raw listener got %d bytes, want %d", len(audio), audiofeed.MonoFrameBytes)
		}
	}

	for i := range uint32(frames) {
		seq, audio, event, err := compressed.Next()
		if err != nil {
			t.Fatalf("the compressed listener failed on frame %d: %v", i, err)
		}
		if event != nil {
			continue
		}
		if seq != i {
			t.Fatalf("the compressed listener got frame %d, want %d", seq, i)
		}
		if len(audio) >= audiofeed.MonoFrameBytes {
			t.Fatalf("the compressed listener got %d bytes, which is not compressed", len(audio))
		}
	}

	// Neither listener took the card from the other, and neither opening it a
	// second time went unnoticed.
	if opened.closed {
		t.Error("the sound card was given back while both listeners were still on it")
	}
}

// TestAudioSideOpensOnceForEveryListener is the reason the daemon holds the
// sound card at all: several things wanting the same audio must not mean several
// things opening the same device.
func TestAudioSideOpensOnceForEveryListener(t *testing.T) {
	var opens int
	side := newAudioSide("Test Line In", audiofeed.ChannelMix, slog.New(slog.DiscardHandler))
	side.open = func(opts audiofeed.Options, out audiofeed.Publisher) (audioCapture, error) {
		opens++
		return &fakeCapture{source: opts.Source, channel: opts.Channel}, nil
	}

	subs := make([]*audiofeed.Sub, 3)
	for i := range subs {
		sub, capture, err := side.acquire()
		if err != nil {
			t.Fatalf("listener %d: %v", i, err)
		}
		if capture.Source() != "Test Line In" {
			t.Errorf("listener %d was given %q", i, capture.Source())
		}
		subs[i] = sub
	}

	if opens != 1 {
		t.Errorf("three listeners opened the sound card %d times, want 1", opens)
	}

	// And all three hear the same frame.
	side.feed.Publish(pcmFrame(1, 9))
	for i, sub := range subs {
		select {
		case f := <-sub.Frames():
			if f.Seq != 1 {
				t.Errorf("listener %d got frame %d, want 1", i, f.Seq)
			}
		default:
			t.Errorf("listener %d got nothing", i)
		}
	}
}

// TestAudioSideKeepsTheCardWhileAnybodyIsListening: the card is given back when
// the last listener leaves, and not before.
func TestAudioSideKeepsTheCardWhileAnybodyIsListening(t *testing.T) {
	var capture *fakeCapture
	side := newAudioSide("Test Line In", audiofeed.ChannelMix, slog.New(slog.DiscardHandler))
	side.open = func(opts audiofeed.Options, out audiofeed.Publisher) (audioCapture, error) {
		capture = &fakeCapture{source: opts.Source}
		return capture, nil
	}

	a, _, err := side.acquire()
	if err != nil {
		t.Fatal(err)
	}
	b, _, err := side.acquire()
	if err != nil {
		t.Fatal(err)
	}

	side.release(a)
	if capture.closed {
		t.Error("the sound card was given back while somebody was still listening")
	}

	side.release(b)

	// The card lingers rather than closing at once, so a listener reconnecting does
	// not reopen it. Nothing here waits out audioLinger; what is checked is
	// that it is still open immediately afterwards.
	if capture.closed {
		t.Error("the sound card was given back at once rather than lingering")
	}

	side.stop()
	if !capture.closed {
		t.Error("the sound card was not given back when the daemon stopped")
	}
}

// TestAudioRefusedWithoutASoundCard: a daemon started without --audio has none
// to give, and says so rather than failing in some less obvious way.
func TestAudioRefusedWithoutASoundCard(t *testing.T) {
	if side := newAudioSide("", audiofeed.ChannelAuto, nil); side != nil {
		t.Fatal("a daemon with no input named still built an audio side")
	}

	var none *audioSide
	if _, _, err := none.acquire(); err == nil {
		t.Fatal("a daemon with no sound card handed out a listener")
	}
	if none.name() != "" {
		t.Error("a daemon with no sound card named one")
	}
}

// TestAudioSideGivesTheCardBackAfterTheLinger: once the last listener has gone
// and the wait is over, the sound card is actually handed back rather than held
// open forever. The linger is shortened for the length of this test so the path
// runs in a moment rather than half a minute.
func TestAudioSideGivesTheCardBackAfterTheLinger(t *testing.T) {
	was := audioLinger
	t.Cleanup(func() { audioLinger = was })
	audioLinger = 5 * time.Millisecond

	var capture *fakeCapture
	side := newAudioSide("Test Line In", audiofeed.ChannelMix, slog.New(slog.DiscardHandler))
	side.open = func(opts audiofeed.Options, out audiofeed.Publisher) (audioCapture, error) {
		capture = &fakeCapture{source: opts.Source}
		return capture, nil
	}

	sub, _, err := side.acquire()
	if err != nil {
		t.Fatal(err)
	}
	side.release(sub)

	closed := func() bool {
		side.mu.Lock()
		defer side.mu.Unlock()
		return capture.closed
	}

	deadline := time.Now().Add(2 * time.Second)
	for !closed() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}

	if !closed() {
		t.Fatal("the sound card was still open after the linger ran out")
	}

	side.mu.Lock()
	defer side.mu.Unlock()
	if side.capture != nil || side.feed != nil || side.closing != nil {
		t.Error("the audio side still points at a capture it gave back")
	}
}

// TestAudioSideKeepsTheCardWhenAListenerCameBack: the wait exists so that a
// listener reconnecting finds the card still open, so the countdown firing after
// somebody has arrived must leave that listener's card alone.
func TestAudioSideKeepsTheCardWhenAListenerCameBack(t *testing.T) {
	was := audioLinger
	t.Cleanup(func() { audioLinger = was })
	audioLinger = time.Hour

	var capture *fakeCapture
	side := newAudioSide("Test Line In", audiofeed.ChannelMix, slog.New(slog.DiscardHandler))
	side.open = func(opts audiofeed.Options, out audiofeed.Publisher) (audioCapture, error) {
		capture = &fakeCapture{source: opts.Source}
		return capture, nil
	}

	first, _, err := side.acquire()
	if err != nil {
		t.Fatal(err)
	}
	side.release(first)

	second, _, err := side.acquire()
	if err != nil {
		t.Fatal(err)
	}

	side.mu.Lock()
	pending := side.closing
	side.mu.Unlock()
	if pending != nil {
		t.Error("a listener arriving during the wait left the countdown armed")
	}

	// What a countdown that fired anyway would do, which is nothing while
	// somebody is listening.
	side.closeIfUnused()
	side.mu.Lock()
	stillOpen := side.capture != nil && !capture.closed
	side.mu.Unlock()
	if !stillOpen {
		t.Fatal("the sound card was given back out from under a listener")
	}

	side.release(second)
	side.stop()

	// Closing an audio side that has already given its card back is what a
	// countdown firing after a shutdown looks like, and it has nothing left to
	// close.
	side.closeIfUnused()
	side.mu.Lock()
	defer side.mu.Unlock()
	if side.capture != nil {
		t.Error("the audio side still points at a capture after it stopped")
	}
}

// TestAudioSideForgetsItsListenersWhenItStops: a daemon shutting down closes
// the sound card, and the count of who was listening has to go with it. Left
// standing, the releases that follow as each stream unwinds count down from a
// number that no longer describes anything, and the next acquire starts from
// below zero and never opens the card again.
func TestAudioSideForgetsItsListenersWhenItStops(t *testing.T) {
	side := newAudioSide("Test Line In", audiofeed.ChannelMix, slog.New(slog.DiscardHandler))
	side.open = func(opts audiofeed.Options, out audiofeed.Publisher) (audioCapture, error) {
		return &fakeCapture{source: opts.Source}, nil
	}

	first, _, err := side.acquire()
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := side.acquire()
	if err != nil {
		t.Fatal(err)
	}

	side.stop()

	side.mu.Lock()
	left := side.holders
	side.mu.Unlock()
	if left != 0 {
		t.Fatalf("%d listeners were left on a card that has been given back", left)
	}

	// The streams unwind after the shutdown, which is the ordinary order: the
	// card is closed and then each listener notices.
	side.release(first)
	side.release(second)

	side.mu.Lock()
	left = side.holders
	side.mu.Unlock()
	if left != 0 {
		t.Fatalf("the count went to %d, want it to stop at nothing", left)
	}

	// A card that opens again proves the count is still usable. Starting from
	// below zero is what stopped it.
	if _, _, err := side.acquire(); err != nil {
		t.Fatalf("the card would not open again: %v", err)
	}
	side.mu.Lock()
	reopened := side.capture != nil && side.holders == 1
	side.mu.Unlock()
	if !reopened {
		t.Fatal("the card did not open again for a listener arriving after a shutdown")
	}
}
