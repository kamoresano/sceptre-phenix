package app

import (
	"errors"
	"fmt"

	"phenix/types"
	"phenix/util/shell"

	"github.com/fatih/color"
)

func init() {
	apps["ntp"] = new(NTP)
	apps["serial"] = new(Serial)
	apps["startup"] = new(Startup)
	apps["user-shell"] = new(UserApp)
	apps["vrouter"] = new(Vrouter)
}

// Action represents the different experiment lifecycle hooks.
type Action string

const (
	ACTIONCONFIG    Action = "configure"
	ACTIONPRESTART  Action = "pre-start"
	ACTIONPOSTSTART Action = "post-start"
	ACTIONCLEANUP   Action = "cleanup"
)

var (
	apps = make(map[string]App)

	defaultApps = map[string]struct{}{
		"ntp":     {},
		"serial":  {},
		"startup": {},
		"vrouter": {},
	}
)

// List returns a list of non-default phenix applications.
func List() []string {
	var names []string

	for name := range apps {
		// Don't include app that wraps external user apps.
		if name == "user-shell" {
			continue
		}

		// Don't include default apps in the list since they always get applied.
		if _, ok := defaultApps[name]; ok {
			continue
		}

		names = append(names, name)
	}

	for _, name := range shell.FindCommandsWithPrefix("phenix-app-") {
		names = append(names, name)
	}

	return names
}

// GetApp returns the initialized phenix app with the given name. If an app with
// the given name is not known internally, it returns the generic `user-shell`
// app that handles shelling out to external custom user apps.
func GetApp(name string) App {
	app, ok := apps[name]
	if !ok {
		app = apps["user-shell"]
	}

	return app
}

// DefaultApps returns a slice of all the initialized default phenix apps.
func DefaultApps() []App {
	var a []App

	for app := range defaultApps {
		a = append(a, apps[app])
	}

	return a
}

// App is the interface that identifies all the required functionality for a
// phenix app. Each experiment lifecycle hook function is passed a pointer to
// the experiment the app is being applied to, and the lifecycle hook function
// should modify the experiment as necessary. Not all lifecycle hook functions
// have to be implemented. If one (or more) isn't needed for an app, it should
// simply return nil.
type App interface {
	// Init is used to initialize a phenix app with options generic to all apps.
	Init(...Option) error

	// Name returns the name of the phenix app.
	Name() string

	// Configure is called for an app at the `configure` experiment lifecycle
	// phase.
	Configure(*types.Experiment) error

	// Start is called for an app at the `pre-start` experiment lifecycle phase.
	PreStart(*types.Experiment) error

	// PostStart is called for an app at the `post-start` experiment lifecycle
	// phase.
	PostStart(*types.Experiment) error

	// Cleanup is called for an app at the `cleanup` experiment lifecycle
	// phase.
	Cleanup(*types.Experiment) error
}

// ApplyApps applies all the default phenix apps and any configured user apps to
// the given experiment for the given lifecycle phase. It returns any errors
// encountered while applying the apps.
func ApplyApps(exp *types.Experiment, opts ...Option) error {
	var (
		options = NewOptions(opts...)
		err     error
	)

	for _, a := range DefaultApps() {
		switch options.Stage {
		case ACTIONCONFIG:
			err = a.Configure(exp)
		case ACTIONPRESTART:
			err = a.PreStart(exp)
		case ACTIONPOSTSTART:
			err = a.PostStart(exp)
		case ACTIONCLEANUP:
			err = a.Cleanup(exp)
		}

		var (
			status  = "✓"
			printer = color.New(color.FgGreen)
		)

		if err != nil {
			status = "✗"
			printer = color.New(color.FgRed)
		}

		printer.Printf("[%s] '%s' default app (%s)\n", status, a.Name(), options.Stage)

		if err != nil {
			return fmt.Errorf("applying default app %s for action %s: %w", a.Name(), options.Stage, err)
		}
	}

	if exp.Spec.Scenario() != nil {
		for _, app := range exp.Spec.Scenario().Apps() {
			// Don't apply default apps again if configured via the Scenario.
			if _, ok := defaultApps[app.Name()]; ok {
				continue
			}

			a := GetApp(app.Name())
			a.Init(Name(app.Name()), DryRun(options.DryRun))

			switch options.Stage {
			case ACTIONCONFIG:
				err = a.Configure(exp)
			case ACTIONPRESTART:
				err = a.PreStart(exp)
			case ACTIONPOSTSTART:
				err = a.PostStart(exp)
			case ACTIONCLEANUP:
				err = a.Cleanup(exp)
			}

			var (
				status  = "✓"
				printer = color.New(color.FgGreen)
			)

			if err != nil {
				if errors.Is(err, ErrUserAppNotFound) {
					status = "?"
					printer = color.New(color.FgYellow)
				} else {
					status = "✗"
					printer = color.New(color.FgRed)
				}
			}

			printer.Printf("[%s] '%s' experiment app (%s)\n", status, a.Name(), options.Stage)

			if err != nil {
				if errors.Is(err, ErrUserAppNotFound) {
					continue
				}

				return fmt.Errorf("applying experiment app %s for action %s: %w", a.Name(), options.Stage, err)
			}
		}
	}

	if options.Stage == ACTIONCONFIG || options.Stage == ACTIONPRESTART {
		// just in case one of the apps added some nodes to the topology...
		exp.Spec.SetDefaults()
	}

	return nil
}
