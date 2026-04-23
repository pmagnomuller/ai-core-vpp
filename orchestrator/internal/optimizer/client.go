// Package optimizer wraps the generated gRPC client so the HTTP layer stays
// free of protobuf types.
package optimizer

import (
	"context"

	pb "github.com/ai-core-vpp/orchestrator/gen/optimizerpb"
)

type TelemetryPoint struct {
	Ts              string  `json:"ts"`
	KwUsage         float64 `json:"kw_usage"`
	BatterySocPct   float64 `json:"battery_soc_pct"`
	HeatpumpStatus  string  `json:"heatpump_status"`
	GridPriceEurKwh float64 `json:"grid_price_eur_kwh"`
}

type Action struct {
	When string `json:"when"`
	What string `json:"what"`
	Why  string `json:"why"`
}

type Plan struct {
	Strategy string   `json:"strategy"`
	Actions  []Action `json:"actions"`
}

type Client struct {
	grpc pb.OptimizerClient
}

func NewClient(c pb.OptimizerClient) *Client {
	return &Client{grpc: c}
}

func (c *Client) OptimizeStrategy(ctx context.Context, deviceID string, history []TelemetryPoint) (*Plan, error) {
	req := &pb.OptimizeRequest{
		DeviceId: deviceID,
		History:  make([]*pb.TelemetryPoint, 0, len(history)),
	}
	for _, p := range history {
		req.History = append(req.History, &pb.TelemetryPoint{
			Ts:              p.Ts,
			KwUsage:         p.KwUsage,
			BatterySocPct:   p.BatterySocPct,
			HeatpumpStatus:  p.HeatpumpStatus,
			GridPriceEurKwh: p.GridPriceEurKwh,
		})
	}
	resp, err := c.grpc.OptimizeStrategy(ctx, req)
	if err != nil {
		return nil, err
	}
	plan := &Plan{Strategy: resp.GetStrategy(), Actions: make([]Action, 0, len(resp.GetActions()))}
	for _, a := range resp.GetActions() {
		plan.Actions = append(plan.Actions, Action{
			When: a.GetWhen(),
			What: a.GetWhat(),
			Why:  a.GetWhy(),
		})
	}
	return plan, nil
}
