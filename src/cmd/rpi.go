package cmd

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
)

var (
	namespace = "rpi"
	vcgencmd  = "vcgencmd"

	temperature = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "temperature",
			Help:      "Temperatures of the components in degree celsius",
		},
		[]string{"sensor", "type"},
	)

	frequency = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "frequency",
			Help:      "Clock frequencies of the components in hertz",
		},
		[]string{"component"},
	)

	voltage = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "voltage",
			Help:      "Voltages of the components in volts",
		},
		[]string{"component"},
	)

	memory = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "memory",
			Help:      "Memory split of CPU and GPU in bytes",
		},
		[]string{"component"},
	)
)

func init() {
	rootCmd.AddCommand(rpiCmd)
	prometheus.MustRegister(temperature)
	prometheus.MustRegister(frequency)
	prometheus.MustRegister(voltage)
	prometheus.MustRegister(memory)
}

type Collector struct{}

func (c *Collector) collectTemperatures() error {
	thermalPath := "/sys/class/thermal"
	sensors, err := os.ReadDir(thermalPath)
	if err != nil {
		return fmt.Errorf("error reading thermal sensors: %v", err)
	}

	for _, sensor := range sensors {
		tempFile := filepath.Join(thermalPath, sensor.Name(), "temp")
		typeFile := filepath.Join(thermalPath, sensor.Name(), "type")

		tempBytes, err := os.ReadFile(tempFile)
		if err != nil {
			continue
		}
		temp, err := strconv.ParseFloat(strings.TrimSpace(string(tempBytes)), 64)
		if err != nil {
			continue
		}

		typeBytes, err := os.ReadFile(typeFile)
		if err != nil {
			continue
		}
		sensorType := strings.TrimSpace(string(typeBytes))

		temperature.WithLabelValues(sensor.Name(), sensorType).Set(temp / 1000.0)
	}
	return nil
}

func (c *Collector) collectFrequencies() error {
	components := []string{"arm", "core", "h264", "isp", "v3d", "uart", "pwm", "emmc", "pixel", "hdmi"}
	for _, comp := range components {
		out, err := exec.Command(vcgencmd, "measure_clock", comp).Output()
		if err != nil {
			continue
		}
		parts := strings.Split(strings.TrimSpace(string(out)), "=")
		if len(parts) != 2 {
			continue
		}
		freq, err := strconv.ParseFloat(parts[1], 64)
		if err != nil {
			continue
		}
		frequency.WithLabelValues(comp).Set(freq)
	}
	return nil
}

func (c *Collector) collectVoltages() error {
	components := []string{"core", "sdram_c", "sdram_i", "sdram_p"}
	for _, comp := range components {
		out, err := exec.Command(vcgencmd, "measure_volts", comp).Output()
		if err != nil {
			continue
		}
		parts := strings.Split(strings.TrimSpace(string(out)), "=")
		if len(parts) != 2 {
			continue
		}
		volts := strings.TrimSuffix(parts[1], "V")
		v, err := strconv.ParseFloat(volts, 64)
		if err != nil {
			continue
		}
		voltage.WithLabelValues(comp).Set(v)
	}
	return nil
}

func (c *Collector) collectMemory() error {
	components := []string{"arm", "gpu"}
	for _, comp := range components {
		out, err := exec.Command(vcgencmd, "get_mem", comp).Output()
		if err != nil {
			continue
		}
		parts := strings.Split(strings.TrimSpace(string(out)), "=")
		if len(parts) != 2 {
			continue
		}
		mem := strings.TrimSuffix(parts[1], "M")
		m, err := strconv.ParseFloat(mem, 64)
		if err != nil {
			continue
		}
		memory.WithLabelValues(comp).Set(m * 1024 * 1024) // Convert MB to bytes
	}
	return nil
}

func (c *Collector) collect() {
	c.collectTemperatures()
	c.collectFrequencies()
	c.collectVoltages()
	c.collectMemory()
}

var rpiCmd = &cobra.Command{
	Use:   "rpi",
	Short: "Start the Raspberry Pi metrics exporter",
	Long: `Start a Prometheus exporter that collects metrics from a Raspberry Pi.
Metrics include temperature, clock frequencies, voltages, and memory split.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		collector := &Collector{}

		// Initial collection
		collector.collect()

		// Start periodic collection
		go func() {
			ticker := time.NewTicker(30 * time.Second)
			for range ticker.C {
				collector.collect()
			}
		}()

		// Expose metrics on /metrics
		http.Handle("/metrics", promhttp.Handler())
		log.Printf("Starting RPI metrics exporter on :9110")
		return http.ListenAndServe(":9110", nil)
	},
}
