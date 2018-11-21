// Copyright © 2018 NAME HERE <EMAIL ADDRESS>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

// clusterListCmd represents the clusterList command
var clusterListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "lists all created clusters",
	Run: func(cmd *cobra.Command, args []string) {
		tw := new(tabwriter.Writer)
		tw.Init(os.Stdout, 0, 8, 2, '\t', 0)
		fmt.Fprintln(tw, "NAME\tNODES\tMASTER IP")

		for _, cluster := range AppConf.Config.Clusters {
			nodes := len(cluster.Nodes)
			var masterIP string
			for _, node := range cluster.Nodes {
				if node.IsMaster {
					masterIP = node.IPAddress
					break
				}
			}
			fmt.Fprintf(tw, "%s\t%d\t%s", cluster.Name, nodes, masterIP)
			fmt.Fprintln(tw)
		}

		tw.Flush()
	},
}

func init() {
	clusterCmd.AddCommand(clusterListCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// clusterListCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// clusterListCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
