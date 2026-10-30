package app

// PreviewAsk is the mod panel asking the dashboard on the stream PC to play
// (or stop) a map's audio preview. N grows with every ask, so the dashboard
// acts once per press and not again after a reload.
type PreviewAsk struct {
	SetID int   `json:"setId"`
	N     int64 `json:"n"`
}

// AskPreview passes a mod panel's preview press to the dashboard.
func (a *App) AskPreview(setID int) {
	a.mu.Lock()
	a.pvAsk = PreviewAsk{SetID: setID, N: a.pvAsk.N + 1}
	a.mu.Unlock()
	a.Changed()
}

func (a *App) PreviewAsked() PreviewAsk {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.pvAsk
}

// SetPreviewing records which set the dashboard is playing a preview of
// (0 = none), so the mod panel can show it.
func (a *App) SetPreviewing(setID int) {
	a.mu.Lock()
	a.previewing = setID
	a.mu.Unlock()
}

func (a *App) Previewing() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.previewing
}
