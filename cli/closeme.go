// closeme
// Fun little TUI where you try to close the box

package cli

import (
	"flag"
	"os"
	"runtime/pprof"

	"fortio.org/cli"
	"fortio.org/log"
	"fortio.org/terminal/ansipixels"
)

type State struct {
	AP         *ansipixels.AnsiPixels
	MemProfile string
	CurX, CurY int
}

func Main() int {
	truecolorDefault := ansipixels.DetectColorMode().TrueColor
	fTrueColor := flag.Bool("truecolor", truecolorDefault,
		"Use true color (24-bit RGB) instead of 8-bit ANSI colors (default is true if COLORTERM is set)")
	fCpuprofile := flag.String("profile-cpu", "", "write cpu profile to `file`")
	fMemprofile := flag.String("profile-mem", "", "write memory profile to `file`")
	fFPS := flag.Float64("fps", 60, "Frames per second (ansipixels rendering)")
	cli.Main()
	if *fCpuprofile != "" {
		f, err := os.Create(*fCpuprofile)
		if err != nil {
			return log.FErrf("can't open file for cpu profile: %v", err)
		}
		err = pprof.StartCPUProfile(f)
		if err != nil {
			return log.FErrf("can't start cpu profile: %v", err)
		}
		log.Infof("Writing cpu profile to %s", *fCpuprofile)
		defer pprof.StopCPUProfile()
	}
	ap := ansipixels.NewAnsiPixels(*fFPS)
	st := &State{
		AP:         ap,
		MemProfile: *fMemprofile,
	}
	ap.TrueColor = *fTrueColor
	if err := ap.Open(); err != nil {
		return 1 // error already logged
	}
	ap.MouseTrackingOn()
	defer func() {
		ap.MouseTrackingOff()
		ap.Restore()
	}()
	return st.Run()
}

const text = `Click me!`

func (st *State) Draw() {
	ap := st.AP
	ap.StartSyncMode()
	ap.ClearScreen()
	// make sure the box stays on screen.
	startx := st.CurX - len(text)/2 - 1
	if startx < 0 {
		st.CurX += -startx
		startx = 0
	}
	endx := startx + len(text) + 2
	if endx > ap.W {
		st.CurX -= endx - ap.W
		startx = ap.W - len(text) - 2
	}
	starty := st.CurY - 1
	if starty < 0 {
		st.CurY += -starty
		starty = 0
	}
	endy := starty + 3
	if endy > ap.H {
		st.CurY -= endy - ap.H
		starty = ap.H - 3
	}
	ap.DrawRoundBox(startx, starty, len(text)+2, 3)
	ap.WriteAtStr(startx+1, starty+1, text)
	ap.EndSyncMode()
}

func (st *State) Run() int {
	ap := st.AP
	ap.SyncBackgroundColor()
	ap.OnResize = func() error {
		st.CurX = ap.W / 2
		st.CurY = ap.H/2 - 1
		st.Draw()
		return nil
	}
	ap.OnMouse = func() {
		// change state cur.x/y to mouse position and redraw
		st.CurX = ap.Mx
		st.CurY = ap.My - 1
		st.Draw()
	}
	_ = ap.OnResize()   // initial draw.
	ap.AutoSync = false // for cursor to blink on splash screen. remove if not wanted.
	err := ap.FPSTicks(st.Tick)
	if st.MemProfile != "" {
		f, errMP := os.Create(st.MemProfile)
		if errMP != nil {
			return log.FErrf("can't open file for mem profile: %v", errMP)
		}
		errMP = pprof.WriteHeapProfile(f)
		if errMP != nil {
			return log.FErrf("can't write mem profile: %v", errMP)
		}
		log.Infof("Wrote memory profile to %s", st.MemProfile)
		_ = f.Close()
	}
	if err != nil {
		log.Infof("Exiting on %v", err)
		return 1
	}
	return 0
}

func (st *State) Tick() bool {
	if len(st.AP.Data) == 0 {
		return true
	}
	c := st.AP.Data[0]
	switch c {
	case 'q', 'Q', 3: // Ctrl-C
		log.Infof("Exiting on %q", c)
		return false
	default:
		log.Debugf("Input %q...", c)
		// Do something
	}
	return true
}
