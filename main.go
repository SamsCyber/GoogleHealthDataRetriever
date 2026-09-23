package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"strconv"
	"time"

	"google.golang.org/genproto/googleapis/type/date"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/oauth"

	healthpb "GoogleHealthDataRetriever/gen/healthpb"
)

const grpcEndpoint = "health.googleapis.com:443"
const sleepTimeFormat = "sleep.interval.civil_end_time"
const stepUrl = "users/me/dataTypes/steps"
const sleepUrl = "users/me/dataTypes/sleep"

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	csvPath := flag.String("csv", "week_macros.csv", "CSV file to create or append to")
	flag.Parse()

	fmt.Println("Retrieving OAuth 2.0 token...")
	ts, err := getTokenSource(ctx)

	if err != nil {
		log.Fatalf("Failed to retrieve OAuth 2.0 token: %v", err)
	}

	conn, err := grpc.NewClient(grpcEndpoint, grpc.WithTransportCredentials(credentials.NewClientTLSFromCert(nil, "")), grpc.WithPerRPCCredentials(oauth.TokenSource{TokenSource: ts}))
	if err != nil {
		log.Fatalf("Failed to create gRPC client: %v", err)
	}
	defer conn.Close()

	client := healthpb.NewDataPointsServiceClient(conn)

	steps, err := fetchDailySteps(ctx, client)
	if err != nil {
		log.Fatalf("steps: %v", err)
	}
	sleep, err := fetchSleep(ctx, client)
	if err != nil {
		log.Fatalf("sleep: %v", err)
	}

	now := time.Now()
	start := now.AddDate(0, 0, -6)

	var rows [][]string
	for d := start; !d.After(now); d = d.AddDate(0, 0, 1) {
		key := d.Format(time.DateOnly)
		s := sleep[key]

		row := []string{key, strconv.FormatInt(steps[key], 10), "", "", ""}
		if s.Slept > 0 {
			row[2] = s.AsleepAt.Format("15:04")
			row[3] = s.WokeAt.Format("15:04")
			row[4] = fmt.Sprintf("%.1f", s.Slept.Hours())
		}
		rows = append(rows, row)
	}

	title := fmt.Sprintf("%s to %s", start.Format(time.DateOnly), now.Format(time.DateOnly))
	header := []string{"Date", "Steps", "Asleep", "Woke", "Sleep (h)"}

	if err := appendToCSV(*csvPath, title, header, rows); err != nil {
		log.Fatalf("export: %v", err)
	}
}

func fetchDailySteps(ctx context.Context, client healthpb.DataPointsServiceClient) (map[string]int64, error) {

	fmt.Println("Sending DailyRollUpDataPoints RPC...")

	now := time.Now()
	start := now.AddDate(0, 0, -6)
	end := now.AddDate(0, 0, 1)

	resp, err := client.DailyRollUpDataPoints(ctx, &healthpb.DailyRollUpDataPointsRequest{
		Parent: stepUrl,
		Range: &healthpb.CivilTimeInterval{
			Start: &healthpb.CivilDateTime{Date: toDate(start)},
			End:   &healthpb.CivilDateTime{Date: toDate(end)},
		},
		WindowSizeDays: 1,
	})
	if err != nil {
		log.Fatalf("DailyRollUpDataPoints: %v", err)
	}

	fmt.Println("--- Success ---")

	steps := make(map[string]int64)
	for _, rp := range resp.GetRollupDataPoints() {
		d := rp.GetCivilStartTime().GetDate()
		fmt.Printf("%04d-%02d-%02d: %d steps\n",
			d.GetYear(), d.GetMonth(), d.GetDay(), rp.GetSteps().GetCountSum())

		steps[fmt.Sprintf("%04d-%02d-%02d", d.GetYear(), d.GetMonth(), d.GetDay())] = rp.GetSteps().GetCountSum()
	}

	return steps, nil
}

func fetchSleep(ctx context.Context, client healthpb.DataPointsServiceClient) (map[string]sleepSummary, error) {
	fmt.Println("Sending ListDataPoints RPC for sleep data...")

	resp, err := client.ListDataPoints(ctx, &healthpb.ListDataPointsRequest{
		Parent: sleepUrl,
		Filter: lastNDaysCivilFilter(sleepTimeFormat, 7),
	})
	if err != nil {
		log.Fatalf("ListDataPoints: %v", err)
	}

	fmt.Println("--- Success ---")

	sleepData := make(map[string]sleepSummary)

	for _, dp := range resp.GetDataPoints() {

		var sleepSummary sleepSummary
		s := dp.GetSleep()
		sum := dp.GetSleep().GetSummary()

		start := s.GetInterval().GetStartTime().AsTime().Local()
		end := s.GetInterval().GetEndTime().AsTime().Local()

		asleepAt := start.Add(time.Duration(sum.GetMinutesToFallAsleep()) * time.Minute)
		wokeAt := end.Add(-time.Duration(sum.GetMinutesAfterWakeUp()) * time.Minute)
		slept := time.Duration(sum.GetMinutesAsleep()) * time.Minute

		sleepSummary.AsleepAt = asleepAt
		sleepSummary.WokeAt = wokeAt
		sleepSummary.Slept = slept
		sleepData[wokeAt.Format(time.DateOnly)] = sleepSummary

		fmt.Printf("%s  asleep %s  woke %s  (%s asleep)\n",
			wokeAt.Format("Mon 02 Jan"),
			asleepAt.Format("15:04"),
			wokeAt.Format("15:04"),
			slept,
		)
	}

	return sleepData, nil
}

func toDate(t time.Time) *date.Date {
	return &date.Date{Year: int32(t.Year()), Month: int32(t.Month()), Day: int32(t.Day())}
}

func lastNDaysCivilFilter(field string, days int) string {
	now := time.Now()
	start := now.AddDate(0, 0, -(days - 1)).Format(time.DateOnly)
	end := now.AddDate(0, 0, 1).Format(time.DateOnly)
	return fmt.Sprintf(`%s >= "%s" AND %s < "%s"`, field, start, field, end)
}

type sleepSummary struct {
	AsleepAt time.Time
	WokeAt   time.Time
	Slept    time.Duration
}
