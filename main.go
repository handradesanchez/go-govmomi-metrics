package main

import (
    "context"
    "fmt"
    "net/url"
    "os"

    "github.com/vmware/govmomi"
    "github.com/vmware/govmomi/performance"
    "github.com/vmware/govmomi/view"
    "github.com/vmware/govmomi/vim25/mo"
    "github.com/vmware/govmomi/vim25/types"
)

func main() {
    // Step 1: Read environment variables
    vc, user, pass := readEnvVars()

    // Step 2: Concatenate https and sdk to the VCSA_SERVER
    vcURL := formatVCURL(vc)

    // Step 3: Create a vSphere client
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    client := createVSphereClient(ctx, vcURL, user, pass)

    // Step 4: Retrieve VMs
    vms := retrieveVMs(ctx, client)

    // Step 5: Retrieve and display VM metrics
    retrieveAndDisplayMetrics(ctx, client, vms)
}

func readEnvVars() (string, string, string) {
    vc := os.Getenv("VCSA_SERVER")
    user := os.Getenv("QA_VCENTER_USERNAME")
    pass := os.Getenv("QA_VCENTER_PASSWORD")

    if vc == "" || user == "" || pass == "" {
        fmt.Println("Environment variables VCSA_SERVER, QA_VCENTER_USERNAME, and QA_VCENTER_PASSWORD must be set")
        os.Exit(1)
    }

    return vc, user, pass
}

func formatVCURL(vc string) string {
    return fmt.Sprintf("https://%s/sdk", vc)
}

func createVSphereClient(ctx context.Context, vcURL, user, pass string) *govmomi.Client {
    u, err := url.Parse(vcURL)
    if err != nil {
        fmt.Printf("Error parsing URL: %v\n", err)
        os.Exit(1)
    }

    u.User = url.UserPassword(user, pass)

    c, err := govmomi.NewClient(ctx, u, true)
    if err != nil {
        fmt.Printf("Error creating vSphere client: %v\n", err)
        os.Exit(1)
    }

    return c
}

func retrieveVMs(ctx context.Context, client *govmomi.Client) []mo.VirtualMachine {
    m := view.NewManager(client.Client)

    v, err := m.CreateContainerView(ctx, client.ServiceContent.RootFolder, []string{"VirtualMachine"}, true)
    if err != nil {
        fmt.Printf("Error creating container view: %v\n", err)
        os.Exit(1)
    }
    defer v.Destroy(ctx)

    var vms []mo.VirtualMachine
    err = v.Retrieve(ctx, []string{"VirtualMachine"}, []string{"name"}, &vms)
    if err != nil {
        fmt.Printf("Error retrieving virtual machines: %v\n", err)
        os.Exit(1)
    }

    return vms
}

func retrieveAndDisplayMetrics(ctx context.Context, client *govmomi.Client, vms []mo.VirtualMachine) {
    pm := performance.NewManager(client.Client)

    // Define the metric to retrieve
    metricName := "cpu.usagemhz.average"

    // Get the performance counter information
    counterInfo, err := pm.CounterInfoByName(ctx)
    if err != nil {
        fmt.Printf("Error retrieving counter info: %v\n", err)
        os.Exit(1)
    }

    counter, ok := counterInfo[metricName]
    if !ok {
        fmt.Printf("Metric %s not found\n", metricName)
        os.Exit(1)
    }

    for _, vm := range vms {
        query := types.PerfQuerySpec{
            Entity:     vm.Reference(),
            MetricId:   []types.PerfMetricId{{CounterId: counter.Key}},
            IntervalId: 20, // 20 seconds interval
            MaxSample:  1,
        }

        metrics, err := pm.Query(ctx, []types.PerfQuerySpec{query})
        if err != nil {
            fmt.Printf("Error querying performance metrics for VM %s: %v\n", vm.Name, err)
            continue
        }

        for _, baseMetric := range metrics {
            metric, ok := baseMetric.(*types.PerfEntityMetric)
            if !ok {
                fmt.Printf("Error asserting metric type for VM %s\n", vm.Name)
                continue
            }

            for _, value := range metric.Value {
                series, ok := value.(*types.PerfMetricIntSeries)
                if !ok {
                    fmt.Printf("Error asserting metric series type for VM %s\n", vm.Name)
                    continue
                }

                if series.Id.CounterId == counter.Key {
                    fmt.Printf("VM: %s, CPU Usage (MHz): %v\n", vm.Name, series.Value)
                }
            }
        }
    }
}
