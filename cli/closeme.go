// closeme
// Fun little TUI where you try to close the box

package cli

import (
	"flag"
	"math"
	"os"
	"runtime/pprof"
	"time"

	"fortio.org/cli"
	"fortio.org/log"
	"fortio.org/terminal/ansipixels"
	"fortio.org/terminal/ansipixels/tcolor"
)

type State struct {
	AP         *ansipixels.AnsiPixels
	MemProfile string
	CurX, CurY int

	PosX, PosY float64
	VelX, VelY float64

	HasMouse               bool
	MouseX, MouseY         float64
	LastMouseX, LastMouseY float64
	LastMouseAt            time.Time
	MouseSpeed             float64
	MouseAwayX, MouseAwayY float64
	MouseDist              float64
	GameOver               bool
	Clicked                bool
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
	ap.HideCursor()
	defer func() {
		ap.MouseTrackingOff()
		ap.ShowCursor()
		ap.Restore()
	}()
	return st.Run()
}

const (
	text  = `Click me!`
	width = len(text) + 4 // +4 is 2 wide borders on each side
)

func (st *State) arenaBounds() (float64, float64, float64, float64) {
	ap := st.AP
	boxW := width
	leftPad := float64(width / 2)
	rightPad := float64(boxW) - leftPad
	minX := leftPad
	maxX := float64(ap.W) - rightPad
	minY := 1.0
	maxY := float64(ap.H - 2)
	if maxX < minX {
		maxX = minX
	}
	if maxY < minY {
		maxY = minY
	}
	return minX, maxX, minY, maxY
}

func (st *State) clampBallToArena() {
	minX, maxX, minY, maxY := st.arenaBounds()
	if st.PosX < minX {
		st.PosX = minX
	}
	if st.PosX > maxX {
		st.PosX = maxX
	}
	if st.PosY < minY {
		st.PosY = minY
	}
	if st.PosY > maxY {
		st.PosY = maxY
	}
}

func (st *State) syncCursor() {
	st.CurX = int(math.Round(st.PosX))
	st.CurY = int(math.Round(st.PosY))
}

func (st *State) updateMouseVector() {
	if !st.HasMouse {
		st.MouseAwayX = 0
		st.MouseAwayY = 0
		st.MouseDist = math.Inf(1)
		return
	}
	st.MouseAwayX = st.PosX - st.MouseX
	st.MouseAwayY = st.PosY - st.MouseY
	st.MouseDist = math.Hypot(st.MouseAwayX, st.MouseAwayY)
}

func (st *State) recordMouse(mx, my float64, now time.Time) {
	if st.HasMouse {
		dt := now.Sub(st.LastMouseAt).Seconds()
		if dt <= 0 {
			dt = 1.0 / 240.0
		}
		st.MouseSpeed = math.Hypot(mx-st.LastMouseX, my-st.LastMouseY) / dt
	} else {
		st.HasMouse = true
		st.MouseSpeed = 0
	}
	st.LastMouseX = mx
	st.LastMouseY = my
	st.LastMouseAt = now
	st.MouseX = mx
	st.MouseY = my
	st.updateMouseVector()
}

func bounceAxis(pos, vel, minPos, maxPos, bounce float64) (float64, float64) {
	if pos < minPos {
		return minPos + (minPos - pos), -vel * bounce
	}
	if pos > maxPos {
		return maxPos - (pos - maxPos), -vel * bounce
	}
	return pos, vel
}

func (st *State) Draw() {
	ap := st.AP
	ap.ClearScreen()
	if ap.W < width || ap.H < 3 {
		return
	}
	startx := min(max(st.CurX-width/2, 0), max(ap.W-width, 0))
	starty := min(max(st.CurY-1, 0), max(ap.H-3, 0))
	// If the mouse is far: green, close: red, in between: yellow.
	const maxDist = 16.0
	var color tcolor.BasicColor
	switch {
	case st.MouseDist < maxDist/2:
		color = tcolor.Red
	case st.MouseDist < maxDist:
		color = tcolor.Yellow
	default:
		color = tcolor.Green
	}
	ap.DrawColoredBox(startx+1, starty, width-2, 3, color.Background(), true)
	ap.WriteAtStr(startx+2, starty+1, text)
}

func (st *State) Run() int {
	ap := st.AP
	ap.SyncBackgroundColor()
	st.PosX = float64(ap.W) / 2
	st.PosY = float64(ap.H/2 - 1)
	st.syncCursor()
	ap.OnResize = func() error {
		st.clampBallToArena()
		st.syncCursor()
		ap.StartSyncMode()
		st.Draw()
		ap.EndSyncMode()
		return nil
	}
	ap.OnMouse = func() {
		st.recordMouse(float64(ap.Mx), float64(ap.My-1), time.Now())
		if ap.MouseRelease() {
			st.Clicked = true
			log.LogVf("Mouse released at (%.2f, %.2f) - dist %.2f", st.MouseX, st.MouseY, st.MouseDist)
			if st.MouseDist < 2.0 {
				st.GameOver = true
			}
		}
	}
	_ = ap.OnResize() // initial draw.
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
	if st.GameOver {
		// Get one more click while "You Won!" is on the screen, so that it doesn't immediately disappear.
		ap.MouseTrackingOff()
		ap.MouseClickOn()
		ap.OnMouse = nil
		ap.Out.Flush()
		_ = ap.ReadOrResizeOrSignal()
		ap.MouseClickOff()
	}
	return 0
}

func (st *State) applyMousePhysics() {
	if !st.HasMouse {
		return
	}
	awayX := st.MouseAwayX
	awayY := st.MouseAwayY
	dist := st.MouseDist
	if dist < 0.001 {
		awayX, awayY, dist = 1, 0, 1
	}
	const fleeRadius = 16.0
	if dist < fleeRadius {
		closeness := 1.0 - dist/fleeRadius
		accel := 0.015 + 0.16*closeness*closeness + 0.00009*st.MouseSpeed*closeness
		if accel > 0.22 {
			accel = 0.22
		}
		st.VelX += (awayX / dist) * accel
		st.VelY += (awayY / dist) * accel
	}
	minX, maxX, minY, maxY := st.arenaBounds()
	leftGap := st.PosX - minX
	rightGap := maxX - st.PosX
	topGap := st.PosY - minY
	bottomGap := maxY - st.PosY
	nearestWallGap := min(min(leftGap, rightGap), min(topGap, bottomGap))
	const wallEscapeRadius = 4.0
	if nearestWallGap < wallEscapeRadius && dist < fleeRadius*1.15 {
		centerX := (minX + maxX) / 2
		centerY := (minY + maxY) / 2
		toCenterX := centerX - st.PosX
		toCenterY := centerY - st.PosY
		toCenterDist := math.Hypot(toCenterX, toCenterY)
		if toCenterDist > 0.001 {
			wallCloseness := 1.0 - nearestWallGap/wallEscapeRadius
			mousePressure := 1.0 - math.Min(dist, fleeRadius*1.15)/(fleeRadius*1.15)
			escapeAccel := 0.03 + 0.22*wallCloseness*mousePressure
			st.VelX += (toCenterX / toCenterDist) * escapeAccel
			st.VelY += (toCenterY / toCenterDist) * escapeAccel
		}
	}
	st.MouseSpeed *= 0.9
}

func (st *State) Tick() bool {
	if len(st.AP.Data) > 0 {
		c := st.AP.Data[0]
		switch c {
		case 'q', 'Q', 3: // Ctrl-C
			log.Infof("Exiting on %q", c)
			return false
		default:
			log.Debugf("Input %q...", c)
		}
	}
	if st.GameOver {
		st.AP.ClearScreen()
		st.AP.WriteBoxed(st.AP.H/2-1, "You WON!")
		return false
	}
	st.applyMousePhysics()
	st.VelX *= 0.97
	st.VelY *= 0.97
	if math.Abs(st.VelX) < 0.001 {
		st.VelX = 0
	}
	if math.Abs(st.VelY) < 0.001 {
		st.VelY = 0
	}
	st.PosX += st.VelX
	st.PosY += st.VelY
	minX, maxX, minY, maxY := st.arenaBounds()
	const bounce = 0.82
	st.PosX, st.VelX = bounceAxis(st.PosX, st.VelX, minX, maxX, bounce)
	st.PosY, st.VelY = bounceAxis(st.PosY, st.VelY, minY, maxY, bounce)

	st.updateMouseVector()
	st.syncCursor()
	st.Draw()
	return true
}
