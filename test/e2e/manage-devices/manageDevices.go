package devices

import (
	"context"
	"fmt"
	"math"
	"os"
	"strconv"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	lamassudevice "github.com/lamassuiot/lamassuiot/pkg/device-manager/client"
	devdto "github.com/lamassuiot/lamassuiot/pkg/device-manager/common/dto"
	"github.com/lamassuiot/lamassuiot/test/e2e/utils"
	client "github.com/lamassuiot/lamassuiot/test/e2e/utils/clients"
)

func ManageDevices(scaleIndex int, certPath string, domain string) error {
	var logger log.Logger
	logger = log.NewJSONLogger(os.Stdout)
	logger = level.NewFilter(logger, level.AllowDebug())
	logger = log.With(logger, "ts", log.DefaultTimestampUTC)
	logger = log.With(logger, "caller", log.DefaultCaller)

	var f, _ = os.Create("./test/e2e/manage-devices/GetDevices_" + strconv.Itoa(scaleIndex) + ".csv")
	var f1, _ = os.Create("./test/e2e/manage-devices/GetDevicebyID_" + strconv.Itoa(scaleIndex) + ".csv")
	var f2, _ = os.Create("./test/e2e/manage-devices/GetDeviceLogs_" + strconv.Itoa(scaleIndex) + ".csv")
	var f3, _ = os.Create("./test/e2e/manage-devices/GetDeviceCertHistory_" + strconv.Itoa(scaleIndex) + ".csv")
	var f4, _ = os.Create("./test/e2e/manage-devices/GetDeviceCertbyID_" + strconv.Itoa(scaleIndex) + ".csv")

	devClient, err := client.LamassuDevClient(certPath, domain)
	if err != nil {
		level.Error(logger).Log("err", err)
		return err
	}

	devices, err := LatencyGetDevices(devClient, f)

	err = LatencyGetDevicebyID(devClient, devices[1].Id, f1)
	if err != nil {
		level.Error(logger).Log("err", err)
		return err
	}
	err = LatencyGetDeviceCertHistory(devClient, devices[1].Id, f3)
	if err != nil {
		level.Error(logger).Log("err", err)
		return err
	}
	err = LatencyGetDeviceLogs(devClient, devices[1].Id, f2)
	if err != nil {
		level.Error(logger).Log("err", err)
		return err
	}

	err = LatencyGetDeviceCert(devClient, devices[1].Id, f4)
	if err != nil {
		level.Error(logger).Log("err", err)
		return err
	}
	return nil
}

func LatencyGetDevicebyID(devClient lamassudevice.LamassuDevManagerClient, id string, f *os.File) error {
	var max, min float64
	max = 0
	min = 12
	for k := 0; k < 10; k++ {
		before := time.Now().UnixNano()
		_, err := devClient.GetDeviceById(context.Background(), id)
		if err != nil {
			fmt.Println(err)
			return err
		}
		after := time.Now().UnixNano()
		latency := float64((after - before)) / 1000000000
		max = math.Max(max, latency)
		min = math.Min(min, latency)
	}
	media := (max + min) / 2
	err := utils.WriteDataFile(id, max, min, media, f)
	if err != nil {
		fmt.Println(err)
		return err
	}

	return nil
}

func LatencyGetDeviceCertHistory(devClient lamassudevice.LamassuDevManagerClient, id string, f *os.File) error {
	var max, min float64
	max = 0
	min = 12
	var totalHistory int
	for k := 0; k < 10; k++ {
		before := time.Now().UnixNano()
		history, err := devClient.GetDeviceCertHistory(context.Background(), id)
		if err != nil {
			fmt.Println(err)
			return err
		}
		after := time.Now().UnixNano()
		latency := float64((after - before)) / 1000000000
		max = math.Max(max, latency)
		min = math.Min(min, latency)
		totalHistory = len(history)
	}
	media := (max + min) / 2
	err := utils.WriteDataFile(strconv.Itoa(totalHistory), max, min, media, f)
	if err != nil {
		fmt.Println(err)
		return err
	}

	return nil
}

func LatencyGetDeviceLogs(devClient lamassudevice.LamassuDevManagerClient, id string, f *os.File) error {
	var max, min float64
	max = 0
	min = 12
	var totalLogs int
	for k := 0; k < 10; k++ {
		before := time.Now().UnixNano()
		logs, err := devClient.GetDeviceLogs(context.Background(), id)
		if err != nil {
			fmt.Println(err)
			return err
		}
		after := time.Now().UnixNano()
		latency := float64((after - before)) / 1000000000
		max = math.Max(max, latency)
		min = math.Min(min, latency)
		totalLogs = len(logs)
	}
	media := (max + min) / 2
	err := utils.WriteDataFile(strconv.Itoa(totalLogs), max, min, media, f)
	if err != nil {
		fmt.Println(err)
		return err
	}

	return nil
}

func LatencyGetDeviceCert(devClient lamassudevice.LamassuDevManagerClient, id string, f *os.File) error {
	var max, min float64
	max = 0
	min = 12
	for k := 0; k < 10; k++ {
		before := time.Now().UnixNano()
		_, err := devClient.GetDeviceCert(context.Background(), id)
		if err != nil {
			fmt.Println(err)
			return err
		}
		after := time.Now().UnixNano()
		latency := float64((after - before)) / 1000000000
		max = math.Max(max, latency)
		min = math.Min(min, latency)
	}
	media := (max + min) / 2
	err := utils.WriteDataFile(id, max, min, media, f)
	if err != nil {
		fmt.Println(err)
		return err
	}

	return nil
}

func LatencyGetDevices(devClient lamassudevice.LamassuDevManagerClient, f *os.File) ([]devdto.Device, error) {
	var max, min float64
	max = 0
	min = 12
	var totalDevices int
	var devices []devdto.Device
	for k := 0; k < 10; k++ {
		before := time.Now().UnixNano()
		devs, total, err := devClient.GetDevices(context.Background(), devdto.QueryParameters{Filter: "", Order: devdto.OrderOptions{Order: "DESC", Field: "id"}, Pagination: devdto.PaginationOptions{Page: 1, Offset: 50}})
		if err != nil {
			fmt.Println(err)
			return []devdto.Device{}, err
		}
		after := time.Now().UnixNano()
		latency := float64((after - before)) / 1000000000
		max = math.Max(max, latency)
		min = math.Min(min, latency)
		totalDevices = total
		devices = devs
	}
	media := (max + min) / 2
	err := utils.WriteDataFile(strconv.Itoa(totalDevices), max, min, media, f)
	if err != nil {
		fmt.Println(err)
		return []devdto.Device{}, err
	}

	return devices, nil
}
