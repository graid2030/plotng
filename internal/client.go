package internal

import (
	"encoding/gob"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"plotng/internal/widget"
)

type Client struct {
	app                *tview.Application
	activePlotsTable   *widget.SortedTable
	plotDirsTable      *widget.SortedTable
	destDirsTable      *widget.SortedTable
	archivedPlotsTable *widget.SortedTable

	logTextbox          *tview.TextView
	hosts               []string
	msg                 map[string]*Msg
	archivedTableActive bool
	activeLogs          map[string][]string
	archivedLogs        map[string][]string
	logPlotID           string
}

var httpClient = &http.Client{
	Timeout: 10 * time.Second, // This covers the entire request
}

func (client *Client) ProcessLoop(hostList string) {
	for _, host := range strings.Split(hostList, ",") {
		host = strings.TrimSpace(host)
		if strings.Index(host, ":") < 0 {
			host += ":8484"
		}
		client.hosts = append(client.hosts, host)
	}
	client.msg = map[string]*Msg{}

	gob.Register(Msg{})
	gob.Register(activePlot{})

	client.setupUI()

	go client.processLoop()
	client.app.Run()
}

func (client *Client) processLoop() {
	client.checkServers()
	ticker := time.NewTicker(30 * time.Second)
	for range ticker.C {
		client.checkServers()
	}
}

func (client *Client) checkServers() {
	for _, host := range client.hosts {
		client.checkServer(host)
	}
}

func (client *Client) getServerData(host string) (*Msg, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("http://%s/", host), nil)
	if err != nil {
		return nil, err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var msg Msg
	decoder := gob.NewDecoder(resp.Body)
	err = decoder.Decode(&msg)
	if err != nil {
		return nil, fmt.Errorf("Failed to decode message: %w", err)
	}
	return &msg, nil
}

func (client *Client) checkServer(host string) {
	// Retrieve data on the goroutine thread
	msg, err := client.getServerData(host)

	// Modify UI state on the tview thread.
	client.app.QueueUpdateDraw(func() {
		if err != nil {
			client.logTextbox.SetText("Log (error) ")
			client.logTextbox.SetText(err.Error())
			return
		}
		client.msg[host] = msg
		client.drawActivePlotsTable()
		client.drawPlotDirsTable()
		client.drawDestDirsTable()
		client.drawArchivedPlotsTable()

		log, ok := client.activeLogs[client.logPlotID]
		if !ok {
			log, ok = client.archivedLogs[client.logPlotID]
		}
		if ok {
			client.logTextbox.SetTitle(fmt.Sprintf(" Log (%s) ", shortenPlotID(client.logPlotID)))
			client.logTextbox.SetText(strings.Join(log, ""))
			client.logTextbox.ScrollToEnd()
		}
	})
}

func (client *Client) tabBetweenTables(event *tcell.EventKey) *tcell.EventKey {
	if event.Key() != tcell.KeyTab {
		return event
	}
	if client.activePlotsTable.HasFocus() {
		client.plotDirsTable.SetFocus(client.app)
		return nil
	}
	if client.plotDirsTable.HasFocus() {
		client.destDirsTable.SetFocus(client.app)
		return nil
	}
	if client.destDirsTable.HasFocus() {
		client.archivedPlotsTable.SetFocus(client.app)
		return nil
	}
	if client.archivedPlotsTable.HasFocus() {
		client.app.SetFocus(client.logTextbox)
		return nil
	}
	if client.logTextbox.HasFocus() {
		client.activePlotsTable.SetFocus(client.app)
		return nil
	}
	return event
}

func (client *Client) setupUI() {
	tview.Styles.PrimitiveBackgroundColor = tcell.ColorDefault
	client.activePlotsTable = widget.NewSortedTable()
	client.activePlotsTable.SetSelectable(true)
	client.activePlotsTable.SetBorder(true)
	client.activePlotsTable.SetTitleAlign(tview.AlignLeft)
	client.activePlotsTable.SetTitle(" Active Plots ")
	client.activePlotsTable.SetSelectedStyle(tcell.StyleDefault.Attributes(tcell.AttrReverse))
	client.activePlotsTable.SetSelectionChangedFunc(client.selectActivePlot)
	client.activePlotsTable.SetupFromType(activePlotsData{})
	client.activePlotsTable.SetInputCapture(client.tabBetweenTables)

	client.plotDirsTable = widget.NewSortedTable()
	client.plotDirsTable.SetSelectable(true)
	client.plotDirsTable.SetBorder(true)
	client.plotDirsTable.SetTitleAlign(tview.AlignLeft)
	client.plotDirsTable.SetTitle(" Plot Directories ")
	client.plotDirsTable.SetSelectedStyle(tcell.StyleDefault.Attributes(tcell.AttrReverse))
	client.plotDirsTable.SetupFromType(plotDirData{})
	client.plotDirsTable.SetInputCapture(client.tabBetweenTables)

	client.destDirsTable = widget.NewSortedTable()
	client.destDirsTable.SetSelectable(true)
	client.destDirsTable.SetBorder(true)
	client.destDirsTable.SetTitleAlign(tview.AlignLeft)
	client.destDirsTable.SetTitle(" Dest Directories ")
	client.destDirsTable.SetSelectedStyle(tcell.StyleDefault.Attributes(tcell.AttrReverse))
	client.destDirsTable.SetupFromType(destDirData{})
	client.destDirsTable.SetInputCapture(client.tabBetweenTables)

	client.archivedPlotsTable = widget.NewSortedTable()
	client.archivedPlotsTable.SetSelectable(true)
	client.archivedPlotsTable.SetBorder(true)
	client.archivedPlotsTable.SetTitleAlign(tview.AlignLeft)
	client.archivedPlotsTable.SetTitle(" Archived Plots ")
	client.archivedPlotsTable.SetSelectedStyle(tcell.StyleDefault.Attributes(tcell.AttrReverse))
	client.archivedPlotsTable.SetSelectionChangedFunc(client.selectArchivedPlot)
	client.archivedPlotsTable.SetupFromType(archivedPlotData{})
	client.archivedPlotsTable.SetInputCapture(client.tabBetweenTables)

	client.logTextbox = tview.NewTextView()
	client.logTextbox.SetBorder(true).SetTitle(" Log ").SetTitleAlign(tview.AlignLeft)
	client.logTextbox.SetInputCapture(client.tabBetweenTables)

	client.logTextbox.ScrollToEnd()

	client.app = tview.NewApplication()

	dirPanel := tview.NewFlex()
	dirPanel.SetDirection(tview.FlexColumn)
	dirPanel.AddItem(client.plotDirsTable, 0, 1, false)
	dirPanel.AddItem(client.destDirsTable, 0, 1, false)

	mainPanel := tview.NewFlex()
	mainPanel.SetDirection(tview.FlexRow)
	mainPanel.AddItem(client.activePlotsTable, 0, 1, true)
	mainPanel.AddItem(dirPanel, 0, 1, false)
	mainPanel.AddItem(client.archivedPlotsTable, 0, 1, false)
	mainPanel.AddItem(client.logTextbox, 0, 1, false)

	client.app = tview.NewApplication()
	client.app.SetRoot(mainPanel, true)
	client.app.EnableMouse(true)
}

func shortenPlotID(id string) string {
	if len(id) < 20 {
		return ""
	}
	return fmt.Sprintf("%s...%s", id[:10], id[len(id)-10:])
}

// Active plots

type activePlotsData struct {
	Host      string        `header:"Host"`
	PlotID    string        `header:"Plot ID"`
	Status    int           `header:"Status"`
	Phase     int           `header:"Phase"    data-align:"right"`
	Progress  int           `header:"Progress" data-align:"right"`
	StartTime time.Time     `header:"Start Time"`
	Duration  time.Duration `header:"Duration"`
	PlotDir   string        `header:"Plot Dir"`
	DestDir   string        `header:"Dest Dir"`
}

func (apd *activePlotsData) Strings() []string {
	status := "Unknown"
	switch apd.Status {
	case plotRunning:
		status = "Running"
	case plotError:
		status = "Errored"
	case plotFinished:
		status = "Finished"
	}
	return []string{
		apd.Host,
		shortenPlotID(apd.PlotID),
		status,
		fmt.Sprintf("%d/4", apd.Phase),
		fmt.Sprintf("%d%%", apd.Progress),
		apd.StartTime.Format("2006-01-02 15:04:05"),
		durationString(apd.Duration),
		apd.PlotDir,
		apd.DestDir,
	}
}

func (client *Client) makeActivePlotsData(host string, p *activePlot) *activePlotsData {
	apd := &activePlotsData{}
	apd.Host = host
	apd.PlotID = p.ID
	apd.Status = p.State
	apd.Phase = p.getCurrentPhase()
	apd.Progress = p.getProgress()
	apd.StartTime = p.getPhaseTime(0)
	apd.Duration = time.Since(apd.StartTime)
	apd.PlotDir = p.PlotDir
	apd.DestDir = p.TargetDir
	return apd
}

func (client *Client) drawActivePlotsTable() {
	activePlotsCount := 0
	client.activeLogs = make(map[string][]string)

	keysToRemove := make(map[string]struct{})
	for _, key := range client.activePlotsTable.Keys() {
		keysToRemove[key] = struct{}{}
	}

	for host, msg := range client.msg {
		for _, plot := range msg.Actives {
			delete(keysToRemove, plot.ID)
			client.activeLogs[plot.ID] = plot.Tail
			client.activePlotsTable.SetRowData(plot.ID, client.makeActivePlotsData(host, plot))
			activePlotsCount++
		}
	}

	for key := range keysToRemove {
		client.activePlotsTable.ClearRowData(key)
	}

	client.activePlotsTable.SetTitle(fmt.Sprintf(" Active Plots [%d] ", activePlotsCount))
}

func (client *Client) selectActivePlot(key string) {
	client.logPlotID = key
	client.archivedPlotsTable.SetSelectedStyle(tcell.StyleDefault.Attributes(tcell.AttrReverse | tcell.AttrDim))
	client.activePlotsTable.SetSelectedStyle(tcell.StyleDefault.Attributes(tcell.AttrReverse | tcell.AttrBold))
	client.logTextbox.SetTitle(fmt.Sprintf(" Log (%s) ", shortenPlotID(client.logPlotID)))
	if log, found := client.activeLogs[key]; found {
		client.logTextbox.SetText(strings.Join(log, ""))
		client.logTextbox.ScrollToEnd()
	} else {
		client.logTextbox.SetText("")
	}
}

// Plot directories

type plotDirData struct {
	Host           string        `header:"Host"`
	PlotDir        string        `header:"Directory"`
	AvailableBytes uint64        `header:"Available Space" data-align:"right"`
	AvgPhase1      time.Duration `header:"Avg Phase 1" data-align:"right"`
	AvgPhase2      time.Duration `header:"Avg Phase 2" data-align:"right"`
	AvgPhase3      time.Duration `header:"Avg Phase 3" data-align:"right"`
	AvgPhase4      time.Duration `header:"Avg Phase 4" data-align:"right"`
	Count          int           `header:"Count" data-align:"right"`
	Failed         int           `header:"Failed" data-align:"right"`
}

func (pdd *plotDirData) Strings() []string {
	return []string{
		pdd.Host,
		pdd.PlotDir,
		spaceString(pdd.AvailableBytes),
		durationString(pdd.AvgPhase1),
		durationString(pdd.AvgPhase2),
		durationString(pdd.AvgPhase3),
		durationString(pdd.AvgPhase4),
		fmt.Sprintf("%d", pdd.Count),
		fmt.Sprintf("%d", pdd.Failed),
	}
}

func (client *Client) makePlotDirsData() map[string]*plotDirData {
	plotDirs := make(map[string]*plotDirData)

	for host, msg := range client.msg {
		for plotDir, plotSpace := range msg.TempDirs {
			plotDirs[host+"||"+plotDir] = &plotDirData{
				Host:           host,
				PlotDir:        plotDir,
				AvailableBytes: plotSpace,
			}
		}

		for _, plot := range msg.Archived {
			pdd, ok := plotDirs[host+"||"+plot.PlotDir]
			if !ok {
				// There's data from a completed plot, but we're no longer using it
				pdd = &plotDirData{
					Host:           host,
					PlotDir:        plot.PlotDir,
					AvailableBytes: math.MaxUint64,
				}
				plotDirs[host+"||"+plot.PlotDir] = pdd
			}
			switch plot.State {
			case plotFinished:
				pdd.AvgPhase1 += plot.getPhaseTime(1).Sub(plot.getPhaseTime(0))
				pdd.AvgPhase2 += plot.getPhaseTime(2).Sub(plot.getPhaseTime(1))
				pdd.AvgPhase3 += plot.getPhaseTime(3).Sub(plot.getPhaseTime(2))
				pdd.AvgPhase4 += plot.getPhaseTime(4).Sub(plot.getPhaseTime(3))
				pdd.Count++
			case plotError, plotKilled:
				pdd.Failed++
			}
		}
	}

	for _, pdd := range plotDirs {
		if pdd.Count > 0 {
			pdd.AvgPhase1 /= time.Duration(pdd.Count)
			pdd.AvgPhase2 /= time.Duration(pdd.Count)
			pdd.AvgPhase3 /= time.Duration(pdd.Count)
			pdd.AvgPhase4 /= time.Duration(pdd.Count)
		}
	}
	return plotDirs
}

func (client *Client) drawPlotDirsTable() {
	plotDirs := client.makePlotDirsData()

	keysToRemove := make(map[string]struct{})
	for _, key := range client.plotDirsTable.Keys() {
		keysToRemove[key] = struct{}{}
	}

	for key, pdd := range plotDirs {
		delete(keysToRemove, key)
		client.plotDirsTable.SetRowData(key, pdd)
	}

	for key := range keysToRemove {
		client.plotDirsTable.ClearRowData(key)
	}

	client.plotDirsTable.SetTitle(fmt.Sprintf(" Plot Directories [%d] ", len(plotDirs)))
}

// Dest Directories

type destDirData struct {
	Host           string        `header:"Host"`
	DestDir        string        `header:"Directory"`
	AvailableBytes uint64        `header:"Available Space" data-align:"right"`
	AvgPlotTime    time.Duration `header:"Avg Plot Time" data-align:"right"`
	Count          int           `header:"Count" data-align:"right"`
	Failed         int           `header:"Failed" data-align:"right"`
}

func (ddd *destDirData) Strings() []string {
	return []string{
		ddd.Host,
		ddd.DestDir,
		spaceString(ddd.AvailableBytes),
		durationString(ddd.AvgPlotTime),
		fmt.Sprintf("%d", ddd.Count),
		fmt.Sprintf("%d", ddd.Failed),
	}
}

func (client *Client) makeDestDirsData() map[string]*destDirData {
	destDirs := make(map[string]*destDirData)

	for host, msg := range client.msg {
		for destDir, plotSpace := range msg.TargetDirs {
			destDirs[host+"||"+destDir] = &destDirData{
				Host:           host,
				DestDir:        destDir,
				AvailableBytes: plotSpace,
			}
		}

		for _, plot := range msg.Archived {
			ddd, ok := destDirs[host+"||"+plot.TargetDir]
			if !ok {
				// There's data from a completed plot, but we're no longer using it
				ddd = &destDirData{
					Host:           host,
					DestDir:        plot.TargetDir,
					AvailableBytes: math.MaxUint64,
				}
				destDirs[host+"||"+plot.TargetDir] = ddd
			}
			switch plot.State {
			case plotFinished:
				ddd.AvgPlotTime += plot.getPhaseTime(4).Sub(plot.getPhaseTime(0))
				ddd.Count++
			case plotError, plotKilled:
				ddd.Failed++
			}
		}
	}

	for _, ddd := range destDirs {
		if ddd.Count > 0 {
			ddd.AvgPlotTime /= time.Duration(ddd.Count)
		}
	}
	return destDirs
}

func (client *Client) drawDestDirsTable() {
	destDirs := client.makeDestDirsData()

	keysToRemove := make(map[string]struct{})
	for _, key := range client.destDirsTable.Keys() {
		keysToRemove[key] = struct{}{}
	}

	for key, ddd := range destDirs {
		delete(keysToRemove, key)
		client.destDirsTable.SetRowData(key, ddd)
	}

	for key := range keysToRemove {
		client.destDirsTable.ClearRowData(key)
	}

	client.destDirsTable.SetTitle(fmt.Sprintf(" Dest Directories [%d] ", len(destDirs)))
}

// Archived plots

type archivedPlotData struct {
	Host      string        `header:"Host"`
	PlotID    string        `header:"Plot Id"`
	Status    int           `header:"Status"`
	Phase     int           `header:"Phase" data-align:"right"`
	StartTime time.Time     `header:"Start Time"`
	EndTime   time.Time     `header:"End Time"`
	Duration  time.Duration `header:"Duration"`
	PlotDir   string        `header:"Plot Dir"`
	DestDir   string        `header:"Dest Dir"`
}

func (apd *archivedPlotData) Strings() []string {
	status := "Unknown"
	switch apd.Status {
	case plotRunning:
		status = "Running"
	case plotError:
		status = "Errored"
	case plotFinished:
		status = "Finished"
	}
	return []string{
		apd.Host,
		shortenPlotID(apd.PlotID),
		status,
		fmt.Sprintf("%d/4", apd.Phase),
		apd.StartTime.Format("2006-01-02 15:04:05"),
		apd.EndTime.Format("2006-01-02 15:04:05"),
		durationString(apd.Duration),
		apd.PlotDir,
		apd.DestDir,
	}
}

func (client *Client) makeArchivedPlotData(host string, p *activePlot) *archivedPlotData {
	apd := &archivedPlotData{}
	apd.Host = host
	apd.PlotID = p.ID
	apd.Status = p.State
	apd.Phase = p.getCurrentPhase()
	apd.StartTime = p.getPhaseTime(0)
	apd.EndTime = p.getPhaseTime(4)
	apd.Duration = apd.EndTime.Sub(apd.StartTime)
	apd.PlotDir = p.PlotDir
	apd.DestDir = p.TargetDir
	return apd
}

func (client *Client) drawArchivedPlotsTable() {
	archivedPlotsSuccess := 0
	archivedPlotsFailed := 0
	client.archivedLogs = make(map[string][]string)

	keysToRemove := make(map[string]struct{})
	for _, key := range client.archivedPlotsTable.Keys() {
		keysToRemove[key] = struct{}{}
	}

	for host, msg := range client.msg {
		for _, plot := range msg.Archived {
			delete(keysToRemove, plot.ID)
			client.archivedLogs[plot.ID] = plot.Tail
			client.archivedPlotsTable.SetRowData(plot.ID, client.makeArchivedPlotData(host, plot))
			switch plot.State {
			case plotFinished:
				archivedPlotsSuccess++
			case plotKilled:
				archivedPlotsFailed++
			case plotError:
				archivedPlotsFailed++
			}
		}
	}

	for key := range keysToRemove {
		client.archivedPlotsTable.ClearRowData(key)
	}

	if archivedPlotsFailed > 0 {
		client.archivedPlotsTable.SetTitle(fmt.Sprintf(" Archived Plots [%d (%d failed)] ", archivedPlotsSuccess, archivedPlotsFailed))
	} else {
		client.archivedPlotsTable.SetTitle(fmt.Sprintf(" Archived Plots [%d] ", archivedPlotsSuccess))
	}
}

func (client *Client) selectArchivedPlot(key string) {
	client.logPlotID = key
	client.activePlotsTable.SetSelectedStyle(tcell.StyleDefault.Attributes(tcell.AttrReverse | tcell.AttrDim))
	client.archivedPlotsTable.SetSelectedStyle(tcell.StyleDefault.Attributes(tcell.AttrReverse | tcell.AttrBold))
	client.logTextbox.SetTitle(fmt.Sprintf(" Log (%s) ", shortenPlotID(client.logPlotID)))
	if log, found := client.archivedLogs[key]; found {
		client.logTextbox.SetText(strings.Join(log, ""))
		client.logTextbox.ScrollToEnd()
	} else {
		client.logTextbox.SetText("")
	}
}
