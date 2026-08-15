// Copyright 2026 Alan Beebe (radiocli.com). All Rights Reserved.
// Author: Alan Beebe
// Created: 8/2/2026

package broker

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/albeebe/radiocli/internal/audiofeed"
	"github.com/albeebe/radiocli/internal/opusenc"
)

// newAudioSide returns the audio half of a daemon, or nil if it has none.
//
// Parameters:
//   - source: name of the sound input this daemon holds, empty for a daemon holding none
//   - channel: which side of the cable the scanner is on, or "auto" to work it out
//   - log: the daemon's own logger, kept for the life of the audio side
//
// Returns:
//   - the audio half of a daemon, or nil when source is empty
func newAudioSide(source, channel string, log *slog.Logger) *audioSide {
	if source == "" {
		return nil
	}
	return &audioSide{
		source:  source,
		channel: channel,
		log:     log,
		open:    audiofeedStart,
	}
}

// acquire adds a listener, opening the sound card if this is the first.
//
// Returns:
//   - a subscription carrying the frames and the news this listener hears
//   - the running capture, for the source and channel it settled on
//   - error if this daemon holds no sound input, or the device would not open
//
// Errors:
//   - errNoAudio: if this daemon was started without a sound input
func (a *audioSide) acquire() (*audiofeed.Sub, audioCapture, error) {
	if a == nil {
		return nil, nil, errNoAudio
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	// A listener arriving during the wait after the last one left takes the
	// card that is still open, which is the whole point of lingering.
	if a.closing != nil {
		a.closing.Stop()
		a.closing = nil
	}

	if a.capture == nil {
		feed := audiofeed.New(a.log)
		capture, err := a.open(audiofeed.Options{
			Source:  a.source,
			Channel: a.channel,
			Log:     a.log,
		}, feed)
		if err != nil {
			return nil, nil, err
		}
		a.feed, a.capture = feed, capture
		a.log.Info("listening to the sound input", "source", capture.Source())
	}

	a.holders++
	return a.feed.Subscribe(audioQueue), a.capture, nil
}

// audio answers OpAudio and, if it can, turns this connection into a stream.
//
// It runs on the connection's own read goroutine up to the point the stream
// starts, so that the reply and the switch to frames happen before the next
// message on this connection is read. Everything after that is the streaming
// goroutine's.
//
// Parameters:
//   - req: the OpAudio message, whose Format and Bitrate choose the tier
func (c *conn) audio(req Request) {
	if c.streaming.Load() {
		c.audioControl(req)
		return
	}

	// An action is for a connection that is already streaming, so it is refused
	// here rather than falling through. Falling through is what it used to do,
	// which turned a request to change the bitrate into a subscription at the
	// default format: the connection became a stream of audio the client never
	// asked for and could no longer send ordinary messages on.
	if req.Action != "" {
		c.send(Response{Type: TypeError, ID: req.ID,
			Error: fmt.Sprintf("%q is for a connection that is already carrying audio: "+
				"ask for audio first, with no action", req.Action)})
		return
	}

	format := req.Format
	if format == "" {
		format = FormatPCM
	}
	if format != FormatPCM && format != FormatOpus {
		c.send(Response{Type: TypeError, ID: req.ID,
			Error: fmt.Sprintf("there is no audio format called %q: it is %q or %q",
				req.Format, FormatPCM, FormatOpus)})
		return
	}

	bitrate := req.Bitrate
	if bitrate == 0 {
		bitrate = defaultBitrate
	}

	var enc *opusenc.Encoder
	if format == FormatOpus {
		var err error
		if enc, err = opusenc.New(bitrate); err != nil {
			c.send(Response{Type: TypeError, ID: req.ID, Error: err.Error()})
			return
		}
	}

	sub, capture, err := c.srv.audio.acquire()
	if err != nil {
		c.send(Response{Type: TypeError, ID: req.ID, Error: err.Error()})
		return
	}

	// The reply and the switch to frames go together, under the one lock every
	// write on this connection takes. Anything else on this socket is either
	// entirely before the newline or entirely after it, which is what stops a
	// stray error line landing in the middle of the audio.
	c.write.Lock()
	err = c.sendLine(Response{
		Type: TypeAudio, ID: req.ID,
		Format: format, Rate: audiofeed.SampleRate, Channels: 1, FrameMS: audiofeed.FrameMS,
		Bitrate: bitrate, Source: capture.Source(), Channel: capture.Channel(),
	})
	if err == nil {
		c.streaming.Store(true)
	}
	c.write.Unlock()

	if err != nil {
		c.srv.audio.release(sub)
		return
	}

	c.streams.Add(1)
	go func() {
		defer c.streams.Done()
		defer c.srv.audio.release(sub)
		c.pumpAudio(sub, enc)
	}()
}

// audioControl handles a message sent on a connection that is already
// streaming.
//
// Answers go back as frames, because that is all this connection carries now.
// A bitrate that took effect is answered with nothing at all: the packets that
// follow are the answer, and a client that wanted an acknowledgement would be
// waiting on a reply channel that no longer exists.
//
// Parameters:
//   - req: the OpAudio message, whose Action says what is being asked for
func (c *conn) audioControl(req Request) {
	switch req.Action {
	case "bitrate":
		select {
		case c.bitrate <- req.Bitrate:
		default:
			// The last one has not been picked up yet. Dropping this is right:
			// what a listener wants is the rate it asked for last, and that is
			// exactly what is already in the queue behind this one.
		}
	default:
		c.sendFrameJSON(Response{Type: TypeError, ID: req.ID,
			Error: fmt.Sprintf("there is nothing an audio connection can do called %q", req.Action)})
	}
}

// closeIfUnused gives the sound card back, unless somebody came along in the
// meantime.
func (a *audioSide) closeIfUnused() {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.holders > 0 || a.capture == nil {
		return
	}

	a.capture.Close()
	a.capture, a.feed, a.closing = nil, nil, nil
	a.log.Info("stopped listening to the sound input, since nothing is")
}

// name is the sound input this daemon holds, or empty for one holding none.
//
// It has a nil receiver on purpose. A daemon without audio has no audioSide at
// all, and every caller of this is asking "is there any, and what", which is one
// question rather than two.
//
// Returns:
//   - the sound input this daemon holds, and empty for a daemon holding none
func (a *audioSide) name() string {
	if a == nil {
		return ""
	}
	return a.source
}

// pumpAudio moves frames from the feed onto the wire until either end stops.
//
// A write that fails ends the stream rather than being retried. The socket is
// gone, and the connection's read loop is about to find that out for itself.
//
// Parameters:
//   - sub: this listener's subscription to the feed
//   - enc: the encoder to compress with, or nil to send the samples themselves
func (c *conn) pumpAudio(sub *audiofeed.Sub, enc *opusenc.Encoder) {
	// Reused for the life of the stream. Nothing here allocates per frame
	// except the framed write itself, which has to.
	packet := make([]byte, opusenc.MaxPacket)
	body := make([]byte, 4+max(opusenc.MaxPacket, audiofeed.MonoFrameBytes))

	for {
		select {
		case <-c.audioDone:
			return

		case bps := <-c.bitrate:
			if enc == nil {
				c.sendFrameJSON(Response{Type: TypeError,
					Error: "this stream is not compressed, so it has no bitrate to change"})
				continue
			}
			if err := enc.SetBitrate(bps); err != nil {
				c.sendFrameJSON(Response{Type: TypeError, Error: err.Error()})
			}

		case ev, ok := <-sub.Events():
			if !ok {
				return
			}
			if err := c.sendEvent(ev); err != nil {
				return
			}

		case frame, ok := <-sub.Frames():
			if !ok {
				return
			}

			// The frame number goes in front of the audio, in the payload, so
			// that anything relaying this can pass the payload on without
			// looking inside it.
			binary.BigEndian.PutUint32(body, frame.Seq)

			n := copy(body[4:], frame.PCM)
			if enc != nil {
				size, err := enc.Encode(frame.PCM, packet)
				if err != nil {
					c.sendFrameJSON(Response{Type: TypeError, Error: err.Error()})
					return
				}
				n = copy(body[4:], packet[:size])
			}

			c.write.Lock()
			err := writeFrame(c.net, FrameAudio, body[:4+n])
			c.write.Unlock()
			if err != nil {
				return
			}
		}
	}
}

// readFrame reads one framed message.
//
// It takes a *bufio.Reader and not an io.Reader on purpose, and the reason is
// the one bug this design is most likely to ship. An audio connection begins as
// newline JSON and becomes frames, so something has to read the lines first. A
// bufio.Scanner cannot do that job and then hand over: it reads ahead into a
// buffer it does not expose, so the first frames are already inside it when the
// switch happens and are lost without trace. The same reader has to do both,
// and *bufio.Reader is the one that can.
//
// Parameters:
//   - r: the reader that read the lines before the framing changed
//
// Returns:
//   - kind: which sort of frame this is, one of the Frame constants
//   - payload: the frame's contents, without the header
//   - err: the read's own error, or a refusal if the header claims more than MaxFrame
func readFrame(r *bufio.Reader) (kind byte, payload []byte, err error) {
	var head [FrameHeader]byte
	if _, err := io.ReadFull(r, head[:]); err != nil {
		return 0, nil, err
	}

	size := int(head[1])<<16 | int(head[2])<<8 | int(head[3])
	if size > MaxFrame {
		return 0, nil, fmt.Errorf("an audio frame claims %d bytes, over the %d limit", size, MaxFrame)
	}

	payload = make([]byte, size)
	if _, err := io.ReadFull(r, payload); err != nil {
		return 0, nil, err
	}
	return head[0], payload, nil
}

// release drops a listener and starts the clock on closing the card.
//
// The count is not taken below zero. A shutdown closes the card and forgets its
// listeners while their streams are still unwinding, so the releases that
// follow are releases of something already accounted for, and letting them
// count down past nothing would leave a daemon that could never open the card
// again.
//
// Parameters:
//   - sub: the subscription this listener was given by acquire
func (a *audioSide) release(sub *audiofeed.Sub) {
	if a == nil {
		return
	}
	sub.Close()

	a.mu.Lock()
	defer a.mu.Unlock()

	if a.holders > 0 {
		a.holders--
	}
	if a.holders > 0 || a.capture == nil {
		return
	}

	a.closing = time.AfterFunc(audioLinger, a.closeIfUnused)
}

// sendEvent puts one piece of news from the capture on the wire.
//
// Parameters:
//   - ev: what the capture has to say, flattened into the message it becomes
//
// Returns:
//   - error if the write failed, which is the socket having gone
func (c *conn) sendEvent(ev audiofeed.Event) error {
	// Flattened so the kind is the object's own "type", which is the shape
	// every other message on this protocol has.
	out := map[string]any{"type": ev.Kind}
	if fields, ok := ev.Payload.(map[string]any); ok {
		for k, v := range fields {
			out[k] = v
		}
	}

	data, err := json.Marshal(out)
	if err != nil {
		return nil
	}

	c.write.Lock()
	defer c.write.Unlock()
	return writeFrame(c.net, FrameJSON, data)
}

// sendFrameJSON writes one message as a framed JSON payload.
//
// Parameters:
//   - msg: the message to send, which is anything an audio connection has to
//     say that is not audio
func (c *conn) sendFrameJSON(msg Response) {
	data, err := marshalJSON(msg)
	if err != nil {
		return
	}

	c.write.Lock()
	defer c.write.Unlock()
	writeFrame(c.net, FrameJSON, data)
}

// stop closes the sound card whatever is listening, for a daemon shutting down.
//
// The count of listeners goes with the card. Leaving it standing would have the
// releases that follow count down from a number that no longer describes
// anything, and an acquire after those would start from below zero and never
// open the card again. Shutdown is where this runs, so the corruption is latent
// rather than reachable, but a count that outlives what it counts is not an
// invariant worth keeping half of.
func (a *audioSide) stop() {
	if a == nil {
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	if a.closing != nil {
		a.closing.Stop()
		a.closing = nil
	}
	if a.capture != nil {
		a.capture.Close()
		a.capture, a.feed = nil, nil
	}
	a.holders = 0
}

// writeFrame writes one framed message.
//
// The caller holds whatever lock serialises writes on this connection. A frame
// split across two writers is a stream that cannot be resynchronised, because
// nothing in it says where a frame begins.
//
// Parameters:
//   - w: where the frame goes, which is the connection itself
//   - kind: which sort of frame this is, one of the Frame constants
//   - payload: the frame's contents, which may be empty
//
// Returns:
//   - error if the write failed, or a refusal if the payload is over MaxFrame
func writeFrame(w io.Writer, kind byte, payload []byte) error {
	if len(payload) > MaxFrame {
		return fmt.Errorf("an audio frame of %d bytes is over the %d limit", len(payload), MaxFrame)
	}

	// One write rather than a header write and a payload write. Two would let a
	// short write or an error leave half a frame on the wire.
	buf := make([]byte, FrameHeader+len(payload))
	buf[0] = kind
	buf[1] = byte(len(payload) >> 16)
	buf[2] = byte(len(payload) >> 8)
	buf[3] = byte(len(payload))
	copy(buf[FrameHeader:], payload)

	_, err := w.Write(buf)
	return err
}
