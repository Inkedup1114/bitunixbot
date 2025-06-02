package backtest

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/rs/zerolog/log"
)

// Reporter generates backtest reports
type Reporter struct {
	results    *Results
	outputPath string
}

// NewReporter creates a new reporter
func NewReporter(results *Results, outputPath string) *Reporter {
	return &Reporter{
		results:    results,
		outputPath: outputPath,
	}
}

// GenerateReport generates all report formats
func (r *Reporter) GenerateReport() error {
	// Create output directory
	if err := os.MkdirAll(r.outputPath, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Generate different report formats
	if err := r.generateSummary(); err != nil {
		return err
	}

	if err := r.generateTradeLog(); err != nil {
		return err
	}

	if err := r.generateJSONReport(); err != nil {
		return err
	}

	if err := r.generateMetricsReport(); err != nil {
		return err
	}

	return nil
}

// generateSummary generates a human-readable summary
func (r *Reporter) generateSummary() error {
	summaryPath := filepath.Join(r.outputPath, "backtest_summary.txt")
	file, err := os.Create(summaryPath)
	if err != nil {
		return fmt.Errorf("failed to create summary file: %w", err)
	}
	defer file.Close()

	fmt.Fprintf(file, "BACKTEST RESULTS SUMMARY\n")
	fmt.Fprintf(file, "========================\n\n")

	fmt.Fprintf(file, "Time Period: %s to %s\n",
		r.results.StartTime.Format("2006-01-02 15:04:05"),
		r.results.EndTime.Format("2006-01-02 15:04:05"))
	fmt.Fprintf(file, "Duration: %s\n\n", r.results.EndTime.Sub(r.results.StartTime))

	fmt.Fprintf(file, "PERFORMANCE METRICS\n")
	fmt.Fprintf(file, "-------------------\n")
	fmt.Fprintf(file, "Initial Balance: $%.2f\n", r.results.InitialBalance)
	fmt.Fprintf(file, "Final Balance: $%.2f\n", r.results.FinalBalance)
	fmt.Fprintf(file, "Total PnL: $%.2f (%.2f%%)\n",
		r.results.TotalPnL,
		(r.results.TotalPnL/r.results.InitialBalance)*100)
	fmt.Fprintf(file, "Total Commission: $%.2f\n\n", r.results.TotalCommission)

	fmt.Fprintf(file, "TRADING STATISTICS\n")
	fmt.Fprintf(file, "-----------------\n")
	fmt.Fprintf(file, "Total Trades: %d\n", r.results.TotalTrades)
	fmt.Fprintf(file, "Winning Trades: %d\n", r.results.WinningTrades)
	fmt.Fprintf(file, "Losing Trades: %d\n", r.results.LosingTrades)
	fmt.Fprintf(file, "Win Rate: %.2f%%\n", r.results.WinRate*100)
	fmt.Fprintf(file, "Profit Factor: %.2f\n\n", r.results.ProfitFactor)

	fmt.Fprintf(file, "RISK METRICS\n")
	fmt.Fprintf(file, "------------\n")
	fmt.Fprintf(file, "Max Drawdown: %.2f%%\n", r.results.MaxDrawdown)
	fmt.Fprintf(file, "Sharpe Ratio: %.2f\n", r.results.SharpeRatio)

	// Add trade breakdown by symbol
	symbolStats := r.calculateSymbolStats()
	if len(symbolStats) > 0 {
		fmt.Fprintf(file, "\nPERFORMANCE BY SYMBOL\n")
		fmt.Fprintf(file, "--------------------\n")
		for symbol, stats := range symbolStats {
			fmt.Fprintf(file, "%s: %d trades, %.2f%% win rate, $%.2f PnL\n",
				symbol, stats.Count, stats.WinRate*100, stats.PnL)
		}
	}

	log.Info().Str("file", summaryPath).Msg("Summary report generated")
	return nil
}

// generateTradeLog generates a CSV log of all trades
func (r *Reporter) generateTradeLog() error {
	csvPath := filepath.Join(r.outputPath, "trade_log.csv")
	file, err := os.Create(csvPath)
	if err != nil {
		return fmt.Errorf("failed to create trade log: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header
	header := []string{
		"Symbol", "Side", "Entry Time", "Exit Time", "Entry Price",
		"Exit Price", "Size", "PnL", "PnL %", "Commission", "Exit Reason",
	}
	if err := writer.Write(header); err != nil {
		return err
	}

	// Write trades
	for _, trade := range r.results.Trades {
		record := []string{
			trade.Symbol,
			trade.Side,
			trade.EntryTime.Format("2006-01-02 15:04:05"),
			trade.ExitTime.Format("2006-01-02 15:04:05"),
			fmt.Sprintf("%.2f", trade.EntryPrice),
			fmt.Sprintf("%.2f", trade.ExitPrice),
			fmt.Sprintf("%.4f", trade.Size),
			fmt.Sprintf("%.2f", trade.PnL),
			fmt.Sprintf("%.2f", trade.PnLPercent),
			fmt.Sprintf("%.2f", trade.Commission),
			trade.ExitReason,
		}
		if err := writer.Write(record); err != nil {
			return err
		}
	}

	log.Info().Str("file", csvPath).Msg("Trade log generated")
	return nil
}

// generateJSONReport generates a JSON report with all data
func (r *Reporter) generateJSONReport() error {
	jsonPath := filepath.Join(r.outputPath, "backtest_results.json")

	report := map[string]interface{}{
		"summary": map[string]interface{}{
			"start_time":       r.results.StartTime,
			"end_time":         r.results.EndTime,
			"initial_balance":  r.results.InitialBalance,
			"final_balance":    r.results.FinalBalance,
			"total_pnl":        r.results.TotalPnL,
			"total_commission": r.results.TotalCommission,
			"total_trades":     r.results.TotalTrades,
			"winning_trades":   r.results.WinningTrades,
			"losing_trades":    r.results.LosingTrades,
			"win_rate":         r.results.WinRate,
			"profit_factor":    r.results.ProfitFactor,
			"max_drawdown":     r.results.MaxDrawdown,
			"sharpe_ratio":     r.results.SharpeRatio,
		},
		"trades":       r.results.Trades,
		"generated_at": time.Now(),
	}

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	if err := os.WriteFile(jsonPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write JSON report: %w", err)
	}

	log.Info().Str("file", jsonPath).Msg("JSON report generated")
	return nil
}

// generateMetricsReport generates a metrics report for analysis
func (r *Reporter) generateMetricsReport() error {
	metricsPath := filepath.Join(r.outputPath, "metrics_report.csv")
	file, err := os.Create(metricsPath)
	if err != nil {
		return fmt.Errorf("failed to create metrics report: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Calculate cumulative metrics
	metrics := r.calculateCumulativeMetrics()

	// Write header
	header := []string{"Date", "Trades", "Cumulative PnL", "Balance", "Win Rate", "Drawdown"}
	if err := writer.Write(header); err != nil {
		return err
	}

	// Write metrics
	for _, m := range metrics {
		record := []string{
			m.Date.Format("2006-01-02"),
			fmt.Sprintf("%d", m.Trades),
			fmt.Sprintf("%.2f", m.CumulativePnL),
			fmt.Sprintf("%.2f", m.Balance),
			fmt.Sprintf("%.2f", m.WinRate*100),
			fmt.Sprintf("%.2f", m.Drawdown),
		}
		if err := writer.Write(record); err != nil {
			return err
		}
	}

	log.Info().Str("file", metricsPath).Msg("Metrics report generated")
	return nil
}

// SymbolStats holds statistics for a symbol
type SymbolStats struct {
	Count   int
	PnL     float64
	WinRate float64
}

// calculateSymbolStats calculates statistics by symbol
func (r *Reporter) calculateSymbolStats() map[string]*SymbolStats {
	stats := make(map[string]*SymbolStats)

	for _, trade := range r.results.Trades {
		if _, exists := stats[trade.Symbol]; !exists {
			stats[trade.Symbol] = &SymbolStats{}
		}

		s := stats[trade.Symbol]
		s.Count++
		s.PnL += trade.PnL
		if trade.PnL > 0 {
			s.WinRate += 1
		}
	}

	// Calculate win rates
	for _, s := range stats {
		if s.Count > 0 {
			s.WinRate = s.WinRate / float64(s.Count)
		}
	}

	return stats
}

// DailyMetrics holds daily performance metrics
type DailyMetrics struct {
	Date          time.Time
	Trades        int
	CumulativePnL float64
	Balance       float64
	WinRate       float64
	Drawdown      float64
}

// calculateCumulativeMetrics calculates cumulative metrics over time
func (r *Reporter) calculateCumulativeMetrics() []DailyMetrics {
	if len(r.results.Trades) == 0 {
		return nil
	}

	// Group trades by day
	dailyTrades := make(map[string][]Trade)
	for _, trade := range r.results.Trades {
		day := trade.ExitTime.Format("2006-01-02")
		dailyTrades[day] = append(dailyTrades[day], trade)
	}

	// Calculate cumulative metrics
	var metrics []DailyMetrics
	cumulativePnL := 0.0
	balance := r.results.InitialBalance
	peak := balance
	totalTrades := 0
	totalWins := 0

	// Get sorted dates
	var dates []string
	for date := range dailyTrades {
		dates = append(dates, date)
	}
	sort.Strings(dates)

	for _, dateStr := range dates {
		trades := dailyTrades[dateStr]
		date, _ := time.Parse("2006-01-02", dateStr)

		dayPnL := 0.0
		dayWins := 0

		for _, trade := range trades {
			dayPnL += trade.PnL
			totalTrades++
			if trade.PnL > 0 {
				dayWins++
				totalWins++
			}
		}

		cumulativePnL += dayPnL
		balance += dayPnL

		if balance > peak {
			peak = balance
		}

		drawdown := (peak - balance) / peak * 100
		winRate := float64(totalWins) / float64(totalTrades)

		metrics = append(metrics, DailyMetrics{
			Date:          date,
			Trades:        totalTrades,
			CumulativePnL: cumulativePnL,
			Balance:       balance,
			WinRate:       winRate,
			Drawdown:      drawdown,
		})
	}

	return metrics
}

// PrintSummary prints a summary to console
func (r *Reporter) PrintSummary() {
	fmt.Println("\n=== BACKTEST RESULTS ===")
	fmt.Printf("Period: %s to %s\n",
		r.results.StartTime.Format("2006-01-02"),
		r.results.EndTime.Format("2006-01-02"))
	fmt.Printf("Initial Balance: $%.2f\n", r.results.InitialBalance)
	fmt.Printf("Final Balance: $%.2f\n", r.results.FinalBalance)
	fmt.Printf("Total PnL: $%.2f (%.2f%%)\n",
		r.results.TotalPnL,
		(r.results.TotalPnL/r.results.InitialBalance)*100)
	fmt.Printf("Total Trades: %d\n", r.results.TotalTrades)
	fmt.Printf("Win Rate: %.2f%%\n", r.results.WinRate*100)
	fmt.Printf("Profit Factor: %.2f\n", r.results.ProfitFactor)
	fmt.Printf("Max Drawdown: %.2f%%\n", r.results.MaxDrawdown)
	fmt.Printf("Sharpe Ratio: %.2f\n", r.results.SharpeRatio)
	fmt.Println("=======================")
}
