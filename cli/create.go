package cli

import (
	"fmt"

	"strconv"

	"math/rand"

	"errors"

	"github.com/AlecAivazis/survey"
	"github.com/txn2/dmk/cfg"
	"github.com/txn2/dmk/cliutils"
	"github.com/txn2/dmk/driver"
	"github.com/txn2/dmk/migrate"
	"github.com/desertbit/grumble"
)

func init() {
	createCmd := &grumble.Command{
		Name:    "create",
		Help:    "create projects, databases, and migrations",
		Aliases: []string{"add"},
	}

	Cli.AddCommand(createCmd)

	createCmd.AddCommand(&grumble.Command{
		Name:    "project",
		Help:    "create a project",
		Aliases: []string{"p"},
		Run: func(c *grumble.Context) error {
			createProject()
			return nil
		},
	})

	createCmd.AddCommand(&grumble.Command{
		Name:    "database",
		Help:    "create a database",
		Aliases: []string{"db", "d"},
		Run: func(c *grumble.Context) error {
			if ok := activeProjectCheck(); ok {
				createDatabase(cfg.Database{})
			}
			return nil
		},
	})

	createCmd.AddCommand(&grumble.Command{
		Name:    "migration",
		Help:    "create a migration",
		Aliases: []string{"m"},
		Run: func(c *grumble.Context) error {
			if ok := activeProjectCheck(); ok {
				createMigration()
			}
			return nil
		},
	})

	createCmd.AddCommand(&grumble.Command{
		Name:    "tunnel",
		Help:    "create an ssh tunnel",
		Aliases: []string{"t"},
		Run: func(c *grumble.Context) error {
			if ok := activeProjectCheck(); ok {
				createTunnel()
			}
			return nil
		},
	})

}

func createTunnel() {
	name := ""
	namePrompt := &survey.Input{
		Message: "Tunnel Name:",
		Help:    "Human readable name. Ex: `ACME Production Server`",
	}
	survey.AskOne(namePrompt, &name, nil)

	machineName := cliutils.MachineName(name)

	description := ""
	descPrompt := &survey.Input{
		Message: "Tunnel Description:",
		Help:    "Ex: `ACME production server with localhost access to mysql.`",
	}
	survey.AskOne(descPrompt, &description, nil)

	component := cfg.Component{
		Kind:        "Tunnel",
		MachineName: machineName,
		Name:        name,
		Description: description,
	}

	fmt.Printf("Configure local endpoint (this machine):\n")

	randPort := strconv.Itoa(3000 + rand.Intn(3000))
	localEp, err := createEndpoint("Local", "localhost", randPort)
	if err != nil {
		Cli.PrintError(err)
		return
	}

	fmt.Printf("Configure server endpoint (tunnel to):\n")
	serverEp, err := createEndpoint("Server", "", "22")
	if err != nil {
		Cli.PrintError(err)
		return
	}

	authUser := ""
	authUserPrompt := &survey.Input{
		Message: "Server SSH Username",
		Help:    "Username used for server ssh connection.`",
	}
	survey.AskOne(authUserPrompt, &authUser, nil)

	fmt.Printf("Configure remote endpoint (destination):\n")
	remoteEp, err := createEndpoint("Remote", "localhost", "3306")
	if err != nil {
		Cli.PrintError(err)
		return
	}

	tunnelCfg := cfg.Tunnel{
		Component: component,
		Local:     localEp,
		Server:    serverEp,
		Remote:    remoteEp,
		TunnelAuth: cfg.TunnelAuth{
			User: authUser,
		},
	}

	if appState.Project.Tunnels == nil {
		appState.Project.Tunnels = map[string]cfg.Tunnel{}
	}

	appState.Project.Tunnels[machineName] = tunnelCfg
	saved := confirmAndSave(appState.Project.Component.MachineName, appState.Project)
	if saved {
		fmt.Println()
		fmt.Printf("NOTICE: Tunnel %s was saved.\n", name)
	}

}

func createEndpoint(name string, defH string, defP string) (cfg.Endpoint, error) {

	host := ""
	hostPrompt := &survey.Input{
		Message: name + " Host:",
		Default: defH,
	}
	survey.AskOne(hostPrompt, &host, nil)

	port := ""
	portPrompt := &survey.Input{
		Message: name + " Port:",
		Default: defP,
	}
	survey.AskOne(portPrompt, &port, nil)

	portN, err := strconv.Atoi(port)
	if err != nil {
		return cfg.Endpoint{}, err
	}

	endpoint := cfg.Endpoint{
		Host: host,
		Port: portN,
	}

	return endpoint, nil
}

func createDatabase(database cfg.Database) {
	name := ""
	prompt := &survey.Input{
		Message: "Database Name:",
		Help:    "Human readable name. Ex: `ACME Production`",
		Default: database.Component.Name,
	}
	survey.AskOne(prompt, &name, nil)

	if database.Component.Name != "" {
		name = database.Component.Name
	}
	machineName := cliutils.MachineName(name)

	description := ""
	prompt = &survey.Input{
		Message: "Database Description:",
		Help:    "Ex: `ACME production mysql`",
		Default: database.Component.Description,
	}
	survey.AskOne(prompt, &description, nil)

	component := cfg.Component{
		Kind:        "Database",
		MachineName: machineName,
		Name:        name,
		Description: description,
	}

	useTunnel := false
	if database.Tunnel != "" {
		useTunnel = true
	}

	useTunnelPrompt := &survey.Confirm{
		Message: "Does database require a tunnel?",
		Default: useTunnel,
	}
	survey.AskOne(useTunnelPrompt, &useTunnel, nil)

	if useTunnel {
		// get tunnel list
		tunnels := make([]string, 0)

		for k := range appState.Project.Tunnels {
			tunnels = append(tunnels, k)
		}

		tunnelPrompt := &survey.Select{
			Message: "Choose a tunnel:",
			Options: tunnels,
		}
		survey.AskOne(tunnelPrompt, &database.Tunnel, nil)
	}

	// add component to database
	database.Component = component
	// configure the database
	promptSelect := &survey.Select{
		Message: "Choose a database driver:",
		Options: DriverManager.RegisteredDrivers(),
		Default: database.Driver,
	}
	survey.AskOne(promptSelect, &database.Driver, nil)

	// configure the driver
	dbDriver, err := DriverManager.GetNewDriver(database.Driver)
	if err != nil {
		Cli.PrintError(err)
	}

	if database.Configuration == nil {
		database.Configuration = driver.Config{}
	}

	// configuration survey
	dbDriver.ConfigSurvey(database.Configuration, machineName)

	if appState.Project.Databases == nil {
		appState.Project.Databases = map[string]cfg.Database{}
	}

	appState.Project.Databases[machineName] = database
	saved := confirmAndSave(appState.Project.Component.MachineName, appState.Project)
	if saved {
		fmt.Println()
		fmt.Printf("NOTICE: Database %s was saved.\n", name)
	}

}

func createMigration() {
	name := ""
	prompt := &survey.Input{
		Message: "Migrate Name:",
		Help:    "Human readable name. Ex: `Migrate users`",
	}
	survey.AskOne(prompt, &name, nil)

	machineName := cliutils.MachineName(name)

	description := ""
	prompt = &survey.Input{
		Message: "Migration Description:",
		Help:    "Ex: `Migrate all users from the user table.`",
	}
	survey.AskOne(prompt, &description, nil)

	component := cfg.Component{
		Kind:        "Migration",
		MachineName: machineName,
		Name:        name,
		Description: description,
	}

	migration := cfg.Migration{
		Component: component,
	}

	dbChooser(PromptCfg{
		Message: "Choose a SOURCE Database",
		Value:   &migration.SourceDb,
	})

	sdb := appState.Project.Databases[migration.SourceDb]
	sourceDbDriverType := sdb.Driver
	sourceDbDriverCfg := sdb.Configuration
	sourceDbDriver, err := DriverManager.GetNewDriver(sourceDbDriverType)
	if err != nil {
		Cli.PrintError(err)
		return
	}

	sourceDbDriver.Configure(sourceDbDriverCfg)

	foundReplacements := 0

	// does the selected driver use a source query?
	if sourceDbDriver.HasOutQuery() {

		sourceQueryPrompt := &survey.Editor{
			Message: "SOURCE Query:",
			Help:    "Example: `SELECT id, username FROM users WHERE active = ?`",
			// todo: get query examples help from driver
		}
		survey.AskOne(sourceQueryPrompt, &migration.SourceQuery, nil)
	}

	foundReplacements = sourceDbDriver.ArgCount(migration.SourceQuery)
	nArgsStr := ""
	prompt = &survey.Input{
		Message: "Number of Required Arguments (0 for none):",
		Help:    "Ex: `The number of ordered arguments to pass to the query.`",
		Default: fmt.Sprintf("%d", foundReplacements),
	}
	survey.AskOne(prompt, &nArgsStr, func(ans interface{}) error {
		nArgs, err := strconv.Atoi(ans.(string))
		if err != nil {
			return errors.New("value must be an integer")
		}

		migration.SourceQueryNArgs = nArgs
		return nil
	})

	// does the selected driver use a count query?
	if sourceDbDriver.HasCountQuery() {
		sourceQueryCountPrompt := &survey.Editor{
			Message: "SOURCE Count Query:",
			Help:    "Example: `SELECT count(1) as total FROM users WHERE active = ?`",
			// todo: get query examples help from driver, every driver may
		}
		survey.AskOne(sourceQueryCountPrompt, &migration.SourceCountQuery, nil)
	}

	script := false
	promptBool := &survey.Confirm{
		Message: "Does the source data require a script for transformation?",
		Help:    "Use javascript to mutate each record before sending.",
	}
	survey.AskOne(promptBool, &script, nil)

	if script == true {
		scriptPrompt := &survey.Editor{
			Message: "Write transformation in javascript.",
		}
		survey.AskOne(scriptPrompt, &migration.TransformationScript, nil)
	}

	dbChooser(PromptCfg{
		Message: "Choose a DESTINATION Database",
		Value:   &migration.DestinationDb,
	})

	dqPrompt := &survey.Editor{
		Message: "DESTINATION Query:",
		Help: "The destination query is run through a template processor." +
			"\nTemplate directives are for queries that use the source record as" +
			"\nexplicit values in the query. This may be used in combination " +
			"\nwith argument replacement.\n" +
			`Example: INSERT INTO table_name JSON '{"id": "{{.id"}}", "username": "{{.username"}}"}` +
			"\n" + `Example: UPDATE example.table SET a = ?, b = ? WHERE = ""{{.something}}"` +
			"\nsee: https://golang.org/pkg/text/template/" +
			"\nsee: https://godoc.org/github.com/Masterminds/sprig",
		// todo: get examples from driver
	}
	survey.AskOne(dqPrompt, &migration.DestinationQuery, nil)

	if appState.Project.Migrations == nil {
		appState.Project.Migrations = map[string]cfg.Migration{}
	}

	appState.Project.Migrations[machineName] = migration
	saved := confirmAndSave(appState.Project.Component.MachineName, appState.Project)
	if saved {
		fmt.Println()
		fmt.Printf("NOTICE: Migration %s was saved.\n", name)
	}
}

func createProject() {

	name := ""
	prompt := &survey.Input{
		Message: "Project Name:",
	}
	survey.AskOne(prompt, &name, nil)

	machineName := cliutils.MachineName(name)

	description := ""
	prompt = &survey.Input{
		Message: "Project Description:",
	}
	survey.AskOne(prompt, &description, nil)

	component := cfg.Component{
		Kind:        "Project",
		MachineName: machineName,
		Name:        name,
		Description: description,
	}

	project := migrate.Project{
		Component: component,
	}

	saved := confirmAndSave(machineName, project)
	if saved {
		fmt.Println()
		fmt.Printf("NOTICE: Project %s was saved.\n", name)
		SetProject(project)
	}
}
