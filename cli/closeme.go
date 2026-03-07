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
)

type State struct {
	AP         *ansipixels.AnsiPixels
	MemProfile string
	CurX, CurY int

	PosX, PosY float64
	VelX, VelY float64

	MouseX, MouseY         float64
	LastMouseX, LastMouseY float64
	LastMouseAt            time.Time
	MouseSpeed             float64
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

const text = `Click me!`

func (st *State) arenaBounds() (float64, float64, float64, float64) {
	ap := st.AP
	boxW := len(text) + 2
	leftPad := float64(len(text)/2 + 1)
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

func (st *State) Draw() {
	ap := st.AP
	ap.ClearScreen()
	if ap.W < len(text)+2 || ap.H < 3 {
		ap.EndSyncMode()
		return
	}
	startx := max(st.CurX-len(text)/2-1, 0)
	endx := startx + len(text) + 2
	if endx > ap.W {
		startx = max(ap.W-len(text)-2, 0)
	}
	starty := max(st.CurY-1, 0)
	endy := starty + 3
	if endy > ap.H {
		starty = max(ap.H-3, 0)
	}
	ap.DrawRoundBox(startx, starty, len(text)+2, 3)
	ap.WriteAtStr(startx+1, starty+1, text)
}

func (st *State) Run() int {
	ap := st.AP
	ap.SyncBackgroundColor()
	st.PosX = float64(ap.W) / 2
	st.PosY = float64(ap.H/2 - 1)
	st.CurX = int(math.Round(st.PosX))
	st.CurY = int(math.Round(st.PosY))
	ap.OnResize = func() error {
		st.clampBallToArena()
		st.CurX = int(math.Round(st.PosX))
		st.CurY = int(math.Round(st.PosY))
		ap.StartSyncMode()
		st.Draw()
		ap.EndSyncMode()
		return nil
	}
	ap.OnMouse = func() {
		mx := float64(ap.Mx)
		my := float64(ap.My - 1)
		now := time.Now()
		if !st.LastMouseAt.IsZero() {
			dt := now.Sub(st.LastMouseAt).Seconds()
			if dt <= 0 {
				dt = 1.0 / 240.0
			}
			mdx := mx - st.LastMouseX
			mdy := my - st.LastMouseY
			st.MouseSpeed = math.Hypot(mdx, mdy) / dt
		} else {
			st.MouseSpeed = 0
		}

		st.LastMouseX = mx
		st.LastMouseY = my
		st.LastMouseAt = now
		st.MouseX = mx
		st.MouseY = my
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
	return 0
}

func (st *State) applyMousePhysics() {
	if st.LastMouseAt.IsZero() {
		return
	}
	awayX := st.PosX - st.MouseX
	awayY := st.PosY - st.MouseY
	dist := math.Hypot(awayX, awayY)
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
	nearestWallGap := leftGap
	if rightGap < nearestWallGap {
		nearestWallGap = rightGap
	}
	if topGap < nearestWallGap {
		nearestWallGap = topGap
	}
	if bottomGap < nearestWallGap {
		nearestWallGap = bottomGap
	}
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
	bounce := 0.82
	if st.PosX < minX {
		st.PosX = minX + (minX - st.PosX)
		st.VelX = -st.VelX * bounce
	}
	if st.PosX > maxX {
		st.PosX = maxX - (st.PosX - maxX)
		st.VelX = -st.VelX * bounce
	}
	if st.PosY < minY {
		st.PosY = minY + (minY - st.PosY)
		st.VelY = -st.VelY * bounce
	}
	if st.PosY > maxY {
		st.PosY = maxY - (st.PosY - maxY)
		st.VelY = -st.VelY * bounce
	}

	st.CurX = int(math.Round(st.PosX))
	st.CurY = int(math.Round(st.PosY))
	st.Draw()
	return true
}
