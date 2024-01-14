package nixops

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"sort"

	_ "modernc.org/sqlite"

	"github.com/olekukonko/tablewriter"
)

// Append to SchemaVersion slice
func (n *Nixops) AddSchemaVersion(sv SchemaVersion) {
	n.SchemaVersion = append(n.SchemaVersion, sv)
}

func (n *Nixops) AddDeployment(uuid string, attrs map[string]string) {
	n.Deployments = append(n.Deployments, Deployment{UUID: uuid, Attrs: attrs})
}

// Append to Resource slice
func (n *Nixops) AddResource(r Resource) {
	n.Resource = append(n.Resource, r)
}

// Append to ResourceAttr slice
func (n *Nixops) AddResourceAttr(ra ResourceAttr) {
	n.ResourceAttr = append(n.ResourceAttr, ra)
}

func FetchEverything() (nixOps Nixops, err error) {
	db, err := sql.Open("sqlite", "file:/nixops/deployments.nixops")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Fetch SchemaVersion
	rows, err := db.Query("SELECT version FROM SchemaVersion")
	if err != nil {
		return
	}
	for rows.Next() {
		var sv SchemaVersion
		if err = rows.Scan(&sv.Version); err != nil {
			return
		}
		nixOps.SchemaVersion = append(nixOps.SchemaVersion, sv)
	}
	rows.Close()

	// Temporary map for Deployments
	deploymentsMap := make(map[string]*Deployment)

	// Fetch Deployments
	rows, err = db.Query("SELECT uuid FROM Deployments")
	if err != nil {
		return
	}
	for rows.Next() {
		var uuid string
		if err = rows.Scan(&uuid); err != nil {
			return
		}
		deploymentsMap[uuid] = &Deployment{UUID: uuid, Attrs: make(map[string]string)}
	}
	rows.Close()

	// Fetch DeploymentAttrs and associate with Deployments
	rows, err = db.Query("SELECT deployment, name, value FROM DeploymentAttrs")
	if err != nil {
		return
	}
	for rows.Next() {
		var deployment, name, value string
		if err = rows.Scan(&deployment, &name, &value); err != nil {
			return
		}
		if dep, exists := deploymentsMap[deployment]; exists {
			dep.Attrs[name] = value
		}
	}
	rows.Close()

	// Add populated Deployments to Nixops
	for _, deployment := range deploymentsMap {
		nixOps.Deployments = append(nixOps.Deployments, *deployment)
	}

	// Fetch Resources and ResourceAttrs as before (no changes needed)

	return
}

func (n *Nixops) PrintDeployments() {
	// Initialize ignoreMap with the values from ignoreAttrs
	ignoreMap := map[string]bool{
		"configsPath": true,
		"nixPath":     true,
	}

	// Collect all attribute names for the header
	attrNames := make(map[string]struct{})
	for _, d := range n.Deployments {
		for name := range d.Attrs {
			fmt.Println(name)
			if !ignoreMap[name] {
				attrNames[name] = struct{}{}
			}
		}
	}

	// Create sorted list of attribute names for consistent column order
	var headers []string
	for name := range attrNames {
		headers = append(headers, name)
	}
	sort.Strings(headers)

	// Initialize table writer
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader(append([]string{"UUID"}, headers...))

	// Populate table rows
	for _, d := range n.Deployments {
		var row []string
		row = append(row, d.UUID)
		for _, name := range headers {
			row = append(row, d.Attrs[name])
		}
		table.Append(row)
	}

	// Render table
	table.Render()
}
