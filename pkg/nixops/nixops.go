package nixops

import (
	"database/sql"
	"log"
	"os"

	"github.com/olekukonko/tablewriter"
	_ "modernc.org/sqlite"
)

func NewNixOps() (n *Nixops, error error) {
	n = &Nixops{}
	db, err := sql.Open("sqlite", "file:/nixops/deployments.nixops")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Use LEFT JOIN to fetch Deployments and their attributes
	query := `
   SELECT d.uuid, a.name, a.value
   FROM Deployments d
   INNER JOIN DeploymentAttrs a ON d.uuid = a.deployment
   `
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var uuid string
		var name, value sql.NullString // Use sql.NullString for nullable fields

		if err = rows.Scan(&uuid, &name, &value); err != nil {
			return nil, err
		}

		// Convert sql.NullString to string, using empty string if NULL
		var nameStr, valueStr string
		if name.Valid {
			nameStr = name.String
		}
		if value.Valid {
			valueStr = value.String
		}

		n.Deployments = append(n.Deployments, Deployment{UUID: uuid, Name: nameStr, Value: valueStr})
	}
	rows.Close()

	// Fetch Resources and ResourceAttrs as before (adjust as needed)
	return
}

func (n *Nixops) GetLatestDeploymentUUID() string {
	latestDeployment := n.Deployments[len(n.Deployments)-1]
	return latestDeployment.UUID
}

func (n *Nixops) ListDeployments() []Deployment {
	return n.Deployments
}

func (n *Nixops) PrintDeployments() {
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"UUID", "Name", "Value"})

	for _, deploy := range n.Deployments {
		row := []string{deploy.UUID, deploy.Name, deploy.Value}
		table.Append(row)
	}

	table.Render() // Send output
}
