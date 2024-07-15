// Copyright 2024 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"io"
	"iter"
	"maps"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"
)

const (
	Text = "text"
	TSV  = "tsv"
)

var (
	allFormats = []string{Text, TSV}
	allParams  = slices.Collect(maps.Keys(param2Extractor))
)

var (
	outputFormat  = flag.String("format", Text, fmt.Sprintf("output format %v", allFormats))
	applicationRe = flag.String("app", ".*", "application regexp")
	scenarioRe    = flag.String("scenario", ".*", "scenario regexp")
	vary          = flag.String("vary", "", fmt.Sprintf("comma-separated parameters to vary with the format <name>=[<lo>:<hi>]/<steps>; supported parameters: %v", allParams))
)

func main() {
	flag.Parse()
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	// Set up filters.
	appRegexp, err := regexp.Compile(*applicationRe)
	if err != nil {
		return fmt.Errorf("parsing application regexp: %v", err)
	}
	scnRegexp, err := regexp.Compile(*scenarioRe)
	if err != nil {
		return fmt.Errorf("parsing scenario regexp: %v", err)
	}

	// Set up output.
	var (
		writeHeader func()
		writeRecord func(AppProfile, Scenario, float64)
	)
	switch format := *outputFormat; format {
	case Text, TSV:
		var w io.Writer = os.Stdout
		if format == Text {
			tw := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
			defer tw.Flush()
			w = tw
		}
		writeHeader = func() {
			fmt.Fprintf(w, "Application\tGC CPU%%\tScenario\tB_R\tO_R\tB_F\tO_F\tB_S\tC_R\tP_F\t∆CPU%%\n")
			if format == Text {
				fmt.Fprintf(w, "-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\n")
			}
		}
		writeRecord = func(app AppProfile, scenario Scenario, cpuFrac float64) {
			fmt.Fprintf(w, "%s\t%.2f%%\t%s\t%.3f\t%.3f\t%.3f\t%.3f\t%.3f\t%.3f\t%.3f\t%+.2f%%\n",
				app.Name,
				float64(app.GCCPU)/float64(app.TotalCPU)*100,
				scenario.Name,
				scenario.RegionAllocBytesFrac,
				scenario.RegionAllocsFrac,
				scenario.FadeAllocBytesFrac,
				scenario.FadeAllocsFrac,
				scenario.ScannedRegionAllocBytesFrac,
				scenario.RegionScanCostRatio,
				scenario.FadeAllocsPointerDensity,
				cpuFrac*100)
		}
	default:
		return fmt.Errorf("unknown output format %q", *outputFormat)
	}

	// Set up programs to vary some variables.
	var varyProgs []*VaryProgram
	if *vary != "" {
		for _, prog := range strings.Split(*vary, ",") {
			vp, err := parseVaryProgram(prog)
			if err != nil {
				return err
			}
			varyProgs = append(varyProgs, vp)
		}
	}

	// Write output.
	writeHeader()
	for _, app := range AppProfiles {
		if !appRegexp.MatchString(app.Name) {
			continue
		}
		for _, scenario := range Scenarios {
			if !scnRegexp.MatchString(scenario.Name) {
				continue
			}
			if len(varyProgs) != 0 {
				for _, prog := range varyProgs {
					for scenario := range prog.Vary(scenario) {
						writeRecord(app, scenario, deltaCPUFrac(app, scenario))
					}
				}
			} else {
				writeRecord(app, scenario, deltaCPUFrac(app, scenario))
			}
		}
	}
	return nil
}

type VaryProgram struct {
	extractParam func(*Scenario) *float64
	lo, hi       float64
	steps        int
}

func (vp *VaryProgram) Vary(scenario Scenario) iter.Seq[Scenario] {
	return func(yield func(Scenario) bool) {
		p := vp.extractParam(&scenario)
		inc := (vp.hi - vp.lo) / float64(vp.steps)
		for *p = vp.lo; *p <= vp.hi; *p += inc {
			if !yield(scenario) {
				break
			}
		}
	}
}

var param2Extractor = map[string]func(*Scenario) *float64{
	"B_R": func(s *Scenario) *float64 {
		return &s.RegionAllocBytesFrac
	},
	"O_R": func(s *Scenario) *float64 {
		return &s.RegionAllocsFrac
	},
	"B_F": func(s *Scenario) *float64 {
		return &s.FadeAllocBytesFrac
	},
	"O_F": func(s *Scenario) *float64 {
		return &s.FadeAllocsFrac
	},
	"B_S": func(s *Scenario) *float64 {
		return &s.ScannedRegionAllocBytesFrac
	},
	"C_R": func(s *Scenario) *float64 {
		return &s.RegionScanCostRatio
	},
	"P_F": func(s *Scenario) *float64 {
		return &s.FadeAllocsPointerDensity
	},
}

func parseVaryProgram(vp string) (*VaryProgram, error) {
	i := strings.IndexByte(vp, '=')
	if i < 0 {
		return nil, fmt.Errorf("invalid vary program: %q", vp)
	}
	param := vp[:i]
	extract, ok := param2Extractor[param]
	if !ok {
		return nil, fmt.Errorf("invalid vary program: unknown parameter: %s", param)
	}
	vp = vp[i+1:]
	if vp[0] != '[' {
		return nil, fmt.Errorf("invalid vary program: %q", vp)
	}
	vp = vp[1:]
	i = strings.IndexByte(vp, ':')
	if i < 0 {
		return nil, fmt.Errorf("invalid vary program: %q", vp)
	}
	lo, err := strconv.ParseFloat(vp[:i], 64)
	if err != nil {
		return nil, fmt.Errorf("invalid vary program: cannot parse lo: %s", vp[:i])
	}
	vp = vp[i+1:]
	i = strings.IndexByte(vp, ']')
	if i < 0 {
		return nil, fmt.Errorf("invalid vary program: %q", vp)
	}
	hi, err := strconv.ParseFloat(vp[:i], 64)
	if err != nil {
		return nil, fmt.Errorf("invalid vary program: cannot parse hi: %s", vp[:i])
	}
	vp = vp[i+1:]
	if vp[0] != '/' {
		return nil, fmt.Errorf("invalid vary program: %q", vp)
	}
	vp = vp[1:]
	steps, err := strconv.ParseInt(vp, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid vary program: cannot parse steps: %s", vp)
	}
	return &VaryProgram{
		extractParam: extract,
		lo:           lo,
		hi:           hi,
		steps:        int(steps),
	}, nil
}
