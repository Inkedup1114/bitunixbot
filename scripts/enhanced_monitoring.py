#!/usr/bin/env python3
"""
Enhanced Monitoring and Observability Script
Provides comprehensive monitoring, alerting, and observability for the Bitunix trading bot.
"""

import argparse
import json
import logging
import os
import sys
import time
import requests
import subprocess
from datetime import datetime, timedelta
from pathlib import Path
from typing import Dict, List, Optional, Any, Tuple
from dataclasses import dataclass, asdict

import pandas as pd
import numpy as np
from prometheus_client.parser import text_string_to_metric_families

# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(levelname)s - %(message)s',
    handlers=[
        logging.FileHandler('monitoring.log'),
        logging.StreamHandler(sys.stdout)
    ]
)
logger = logging.getLogger(__name__)

@dataclass
class AlertRule:
    """Alert rule configuration."""
    name: str
    metric: str
    condition: str  # 'gt', 'lt', 'eq', 'ne'
    threshold: float
    duration: int  # seconds
    severity: str  # 'critical', 'warning', 'info'
    description: str
    enabled: bool = True

@dataclass
class MetricValue:
    """Metric value with metadata."""
    name: str
    value: float
    labels: Dict[str, str]
    timestamp: datetime
    help_text: str = ""

@dataclass
class Alert:
    """Alert instance."""
    rule_name: str
    metric_name: str
    current_value: float
    threshold: float
    severity: str
    description: str
    timestamp: datetime
    resolved: bool = False

class PrometheusClient:
    """Client for Prometheus metrics."""
    
    def __init__(self, prometheus_url: str = "http://localhost:9090"):
        self.base_url = prometheus_url.rstrip('/')
        self.session = requests.Session()
        self.session.timeout = 30
    
    def query(self, query: str) -> Optional[Dict]:
        """Execute Prometheus query."""
        try:
            response = self.session.get(
                f"{self.base_url}/api/v1/query",
                params={'query': query}
            )
            response.raise_for_status()
            return response.json()
        except Exception as e:
            logger.error(f"Prometheus query failed: {e}")
            return None
    
    def query_range(self, query: str, start: datetime, end: datetime, step: str = "1m") -> Optional[Dict]:
        """Execute Prometheus range query."""
        try:
            response = self.session.get(
                f"{self.base_url}/api/v1/query_range",
                params={
                    'query': query,
                    'start': start.timestamp(),
                    'end': end.timestamp(),
                    'step': step
                }
            )
            response.raise_for_status()
            return response.json()
        except Exception as e:
            logger.error(f"Prometheus range query failed: {e}")
            return None

class MetricsCollector:
    """Collects metrics from bot and system."""
    
    def __init__(self, metrics_url: str = "http://localhost:8080/metrics"):
        self.metrics_url = metrics_url
        self.session = requests.Session()
        self.session.timeout = 30
    
    def collect_bot_metrics(self) -> List[MetricValue]:
        """Collect metrics from bot's Prometheus endpoint."""
        try:
            response = self.session.get(self.metrics_url)
            response.raise_for_status()
            
            metrics = []
            for family in text_string_to_metric_families(response.text):
                for sample in family.samples:
                    metric = MetricValue(
                        name=sample.name,
                        value=sample.value,
                        labels=sample.labels,
                        timestamp=datetime.now()
                    )
                    metrics.append(metric)
            
            logger.info(f"Collected {len(metrics)} bot metrics")
            return metrics
            
        except Exception as e:
            logger.error(f"Failed to collect bot metrics: {e}")
            return []
    
    def collect_system_metrics(self) -> List[MetricValue]:
        """Collect system metrics using psutil."""
        try:
            import psutil
            
            metrics = []
            now = datetime.now()
            
            # CPU metrics
            cpu_percent = psutil.cpu_percent(interval=1)
            metrics.append(MetricValue(
                name="system_cpu_usage_percent",
                value=cpu_percent,
                labels={},
                timestamp=now
            ))
            
            # Memory metrics
            memory = psutil.virtual_memory()
            metrics.append(MetricValue(
                name="system_memory_usage_percent",
                value=memory.percent,
                labels={},
                timestamp=now
            ))
            
            # Disk metrics
            disk = psutil.disk_usage('/')
            metrics.append(MetricValue(
                name="system_disk_usage_percent",
                value=disk.percent,
                labels={},
                timestamp=now
            ))
            
            # Network metrics
            net_io = psutil.net_io_counters()
            metrics.extend([
                MetricValue(
                    name="system_network_bytes_sent",
                    value=net_io.bytes_sent,
                    labels={},
                    timestamp=now
                ),
                MetricValue(
                    name="system_network_bytes_recv",
                    value=net_io.bytes_recv,
                    labels={},
                    timestamp=now
                )
            ])
            
            logger.info(f"Collected {len(metrics)} system metrics")
            return metrics
            
        except Exception as e:
            logger.error(f"Failed to collect system metrics: {e}")
            return []
    
    def collect_trading_metrics(self) -> List[MetricValue]:
        """Collect trading-specific metrics."""
        try:
            # Get bot metrics first
            bot_metrics = self.collect_bot_metrics()
            
            # Filter for trading-related metrics
            trading_metrics = [
                m for m in bot_metrics
                if any(prefix in m.name for prefix in [
                    'orders_', 'trades_', 'pnl_', 'positions_',
                    'ml_', 'vwap_', 'errors_'
                ])
            ]
            
            logger.info(f"Collected {len(trading_metrics)} trading metrics")
            return trading_metrics
            
        except Exception as e:
            logger.error(f"Failed to collect trading metrics: {e}")
            return []
    
    def collect_ml_metrics(self) -> List[MetricValue]:
        """Collect ML-specific metrics."""
        try:
            # Get bot metrics first
            bot_metrics = self.collect_bot_metrics()
            
            # Filter for ML-related metrics
            ml_metrics = [
                m for m in bot_metrics
                if any(prefix in m.name for prefix in [
                    'ml_', 'onnx_', 'model_'
                ])
            ]
            
            logger.info(f"Collected {len(ml_metrics)} ML metrics")
            return ml_metrics
            
        except Exception as e:
            logger.error(f"Failed to collect ML metrics: {e}")
            return []

class AlertManager:
    """Manages alerting rules and notifications."""
    
    def __init__(self, config_path: str = None):
        self.config_path = config_path
        self.rules: List[AlertRule] = []
        self.active_alerts: List[Alert] = []
        self.alert_history: List[Alert] = []
        self.load_rules()
    
    def load_rules(self):
        """Load alert rules from configuration."""
        # Default rules
        default_rules = [
            AlertRule(
                name="high_cpu_usage",
                metric="system_cpu_usage_percent",
                condition="gt",
                threshold=80.0,
                duration=300,  # 5 minutes
                severity="warning",
                description="System CPU usage is high"
            ),
            AlertRule(
                name="critical_cpu_usage",
                metric="system_cpu_usage_percent",
                condition="gt",
                threshold=95.0,
                duration=60,  # 1 minute
                severity="critical",
                description="System CPU usage is critically high"
            ),
            AlertRule(
                name="high_memory_usage",
                metric="system_memory_usage_percent",
                condition="gt",
                threshold=85.0,
                duration=300,
                severity="warning",
                description="System memory usage is high"
            ),
            AlertRule(
                name="ml_prediction_timeout",
                metric="ml_timeouts_total",
                condition="gt",
                threshold=10,
                duration=300,
                severity="critical",
                description="ML prediction timeouts detected"
            ),
            AlertRule(
                name="trading_error_rate",
                metric="trading_errors_total",
                condition="gt",
                threshold=5,
                duration=300,
                severity="critical",
                description="High trading error rate detected"
            ),
            AlertRule(
                name="websocket_disconnections",
                metric="websocket_disconnections_total",
                condition="gt",
                threshold=3,
                duration=600,
                severity="warning",
                description="Frequent WebSocket disconnections"
            ),
            AlertRule(
                name="low_ml_accuracy",
                metric="ml_accuracy",
                condition="lt",
                threshold=0.55,
                duration=1800,  # 30 minutes
                severity="warning",
                description="ML model accuracy is below threshold"
            )
        ]
        
        self.rules = default_rules
        
        # Load custom rules if config file exists
        if self.config_path and Path(self.config_path).exists():
            try:
                with open(self.config_path, 'r') as f:
                    config = json.load(f)
                
                custom_rules = []
                for rule_data in config.get('alert_rules', []):
                    rule = AlertRule(**rule_data)
                    custom_rules.append(rule)
                
                self.rules.extend(custom_rules)
                logger.info(f"Loaded {len(custom_rules)} custom alert rules")
                
            except Exception as e:
                logger.error(f"Failed to load alert rules: {e}")
        
        logger.info(f"Loaded {len(self.rules)} total alert rules")
    
    def evaluate_alerts(self, metrics: List[MetricValue]) -> List[Alert]:
        """Evaluate metrics against alert rules."""
        new_alerts = []
        
        # Create metric lookup
        metric_map = {m.name: m for m in metrics}
        
        for rule in self.rules:
            if not rule.enabled:
                continue
            
            metric = metric_map.get(rule.metric)
            if not metric:
                continue
            
            # Evaluate condition
            triggered = False
            if rule.condition == "gt":
                triggered = metric.value > rule.threshold
            elif rule.condition == "lt":
                triggered = metric.value < rule.threshold
            elif rule.condition == "eq":
                triggered = abs(metric.value - rule.threshold) < 0.001
            elif rule.condition == "ne":
                triggered = abs(metric.value - rule.threshold) >= 0.001
            
            if triggered:
                # Check if alert already exists
                existing_alert = next(
                    (a for a in self.active_alerts if a.rule_name == rule.name and not a.resolved),
                    None
                )
                
                if not existing_alert:
                    alert = Alert(
                        rule_name=rule.name,
                        metric_name=rule.metric,
                        current_value=metric.value,
                        threshold=rule.threshold,
                        severity=rule.severity,
                        description=rule.description,
                        timestamp=datetime.now()
                    )
                    new_alerts.append(alert)
                    self.active_alerts.append(alert)
                    logger.warning(f"Alert triggered: {rule.name} - {rule.description}")
        
        return new_alerts
    
    def resolve_alerts(self, metrics: List[MetricValue]):
        """Resolve alerts that no longer meet conditions."""
        metric_map = {m.name: m for m in metrics}
        
        for alert in self.active_alerts:
            if alert.resolved:
                continue
            
            # Find corresponding rule
            rule = next((r for r in self.rules if r.name == alert.rule_name), None)
            if not rule:
                continue
            
            metric = metric_map.get(alert.metric_name)
            if not metric:
                continue
            
            # Check if condition is no longer met
            resolved = False
            if rule.condition == "gt":
                resolved = metric.value <= rule.threshold
            elif rule.condition == "lt":
                resolved = metric.value >= rule.threshold
            elif rule.condition == "eq":
                resolved = abs(metric.value - rule.threshold) >= 0.001
            elif rule.condition == "ne":
                resolved = abs(metric.value - rule.threshold) < 0.001
            
            if resolved:
                alert.resolved = True
                self.alert_history.append(alert)
                logger.info(f"Alert resolved: {alert.rule_name}")
    
    def send_notifications(self, alerts: List[Alert]):
        """Send notifications for new alerts."""
        for alert in alerts:
            self._send_alert_notification(alert)
    
    def _send_alert_notification(self, alert: Alert):
        """Send notification for a single alert."""
        try:
            # Log alert
            logger.error(f"🚨 ALERT [{alert.severity.upper()}]: {alert.description}")
            logger.error(f"   Metric: {alert.metric_name} = {alert.current_value:.2f} (threshold: {alert.threshold:.2f})")
            
            # Send to Slack (if webhook configured)
            slack_webhook = os.getenv('SLACK_WEBHOOK_URL')
            if slack_webhook:
                self._send_slack_notification(alert, slack_webhook)
            
            # Send email (if configured)
            email_config = self._get_email_config()
            if email_config:
                self._send_email_notification(alert, email_config)
                
        except Exception as e:
            logger.error(f"Failed to send alert notification: {e}")
    
    def _send_slack_notification(self, alert: Alert, webhook_url: str):
        """Send alert notification to Slack."""
        try:
            # Format message
            color = {
                'critical': '#FF0000',
                'warning': '#FFA500',
                'info': '#00FF00'
            }.get(alert.severity, '#808080')
            
            message = {
                "attachments": [{
                    "color": color,
                    "title": f"🚨 {alert.severity.upper()} Alert: {alert.rule_name}",
                    "text": alert.description,
                    "fields": [
                        {
                            "title": "Metric",
                            "value": alert.metric_name,
                            "short": True
                        },
                        {
                            "title": "Current Value",
                            "value": f"{alert.current_value:.2f}",
                            "short": True
                        },
                        {
                            "title": "Threshold",
                            "value": f"{alert.threshold:.2f}",
                            "short": True
                        },
                        {
                            "title": "Time",
                            "value": alert.timestamp.strftime('%Y-%m-%d %H:%M:%S'),
                            "short": True
                        }
                    ]
                }]
            }
            
            # Send to Slack
            response = requests.post(
                webhook_url,
                json=message,
                timeout=10
            )
            response.raise_for_status()
            logger.info(f"Sent Slack notification for alert: {alert.rule_name}")
            
        except Exception as e:
            logger.error(f"Failed to send Slack notification: {e}")
    
    def _get_email_config(self) -> Optional[Dict[str, str]]:
        """Get email configuration from environment variables."""
        required_vars = [
            'SMTP_SERVER',
            'SMTP_PORT',
            'SMTP_USERNAME',
            'SMTP_PASSWORD',
            'ALERT_EMAIL_FROM',
            'ALERT_EMAIL_TO'
        ]
        
        config = {}
        for var in required_vars:
            value = os.getenv(var)
            if not value:
                return None
            config[var.lower()] = value
        
        return config
    
    def _send_email_notification(self, alert: Alert, config: Dict[str, str]):
        """Send alert notification via email."""
        try:
            import smtplib
            from email.mime.text import MIMEText
            from email.mime.multipart import MIMEMultipart
            
            # Create message
            msg = MIMEMultipart()
            msg['From'] = config['alert_email_from']
            msg['To'] = config['alert_email_to']
            msg['Subject'] = f"[{alert.severity.upper()}] Alert: {alert.rule_name}"
            
            # Format body
            body = f"""
            Alert Details:
            -------------
            Rule: {alert.rule_name}
            Severity: {alert.severity}
            Description: {alert.description}
            
            Metric Information:
            ------------------
            Metric: {alert.metric_name}
            Current Value: {alert.current_value:.2f}
            Threshold: {alert.threshold:.2f}
            Time: {alert.timestamp.strftime('%Y-%m-%d %H:%M:%S')}
            
            This is an automated alert from the Bitunix Trading Bot monitoring system.
            """
            
            msg.attach(MIMEText(body, 'plain'))
            
            # Send email
            with smtplib.SMTP(config['smtp_server'], int(config['smtp_port'])) as server:
                server.starttls()
                server.login(config['smtp_username'], config['smtp_password'])
                server.send_message(msg)
            
            logger.info(f"Sent email notification for alert: {alert.rule_name}")
            
        except Exception as e:
            logger.error(f"Failed to send email notification: {e}")

class MonitoringDashboard:
    """Generate monitoring dashboard and reports."""
    
    def __init__(self, metrics_collector: MetricsCollector, alert_manager: AlertManager):
        self.metrics_collector = metrics_collector
        self.alert_manager = alert_manager
    
    def generate_health_report(self) -> Dict[str, Any]:
        """Generate comprehensive health report."""
        logger.info("Generating health report...")
        
        # Collect all metrics
        bot_metrics = self.metrics_collector.collect_bot_metrics()
        system_metrics = self.metrics_collector.collect_system_metrics()
        all_metrics = bot_metrics + system_metrics
        
        # Evaluate alerts
        new_alerts = self.alert_manager.evaluate_alerts(all_metrics)
        self.alert_manager.resolve_alerts(all_metrics)
        
        # Send notifications
        if new_alerts:
            self.alert_manager.send_notifications(new_alerts)
        
        # Build report
        report = {
            'timestamp': datetime.now().isoformat(),
            'status': 'healthy' if not self.alert_manager.active_alerts else 'unhealthy',
            'metrics_count': len(all_metrics),
            'active_alerts': len([a for a in self.alert_manager.active_alerts if not a.resolved]),
            'new_alerts': len(new_alerts),
            'summary': self._generate_summary(all_metrics),
            'alerts': [asdict(a) for a in self.alert_manager.active_alerts if not a.resolved],
            'metrics': [asdict(m) for m in all_metrics]
        }
        
        return report
    
    def _generate_summary(self, metrics: List[MetricValue]) -> Dict[str, Any]:
        """Generate summary of system health."""
        summary = {
            'status': 'healthy',
            'issues': [],
            'warnings': [],
            'metrics_summary': {}
        }
        
        # Create metric lookup
        metric_map = {m.name: m for m in metrics}
        
        # Check system health
        cpu_usage = metric_map.get('system_cpu_usage_percent')
        if cpu_usage and cpu_usage.value > 90:
            summary['issues'].append(f"Critical CPU usage: {cpu_usage.value:.1f}%")
            summary['status'] = 'unhealthy'
        elif cpu_usage and cpu_usage.value > 80:
            summary['warnings'].append(f"High CPU usage: {cpu_usage.value:.1f}%")
        
        memory_usage = metric_map.get('system_memory_usage_percent')
        if memory_usage and memory_usage.value > 90:
            summary['issues'].append(f"Critical memory usage: {memory_usage.value:.1f}%")
            summary['status'] = 'unhealthy'
        elif memory_usage and memory_usage.value > 80:
            summary['warnings'].append(f"High memory usage: {memory_usage.value:.1f}%")
        
        # Check trading health
        error_rate = metric_map.get('errors_total')
        if error_rate and error_rate.value > 10:
            summary['issues'].append(f"High error rate: {error_rate.value} errors")
            summary['status'] = 'unhealthy'
        
        ml_accuracy = metric_map.get('ml_accuracy')
        if ml_accuracy and ml_accuracy.value < 0.6:
            summary['issues'].append(f"Low ML accuracy: {ml_accuracy.value:.2f}")
            summary['status'] = 'unhealthy'
        elif ml_accuracy and ml_accuracy.value < 0.7:
            summary['warnings'].append(f"Degraded ML accuracy: {ml_accuracy.value:.2f}")
        
        # Generate metrics summary
        for metric in metrics:
            if metric.name not in summary['metrics_summary']:
                summary['metrics_summary'][metric.name] = {
                    'value': metric.value,
                    'labels': metric.labels
                }
        
        return summary
    
    def save_report(self, report: Dict[str, Any], output_path: str):
        """Save health report to file."""
        try:
            # Create directory if it doesn't exist
            output_dir = os.path.dirname(output_path)
            if output_dir:
                os.makedirs(output_dir, exist_ok=True)
            
            # Save report
            with open(output_path, 'w') as f:
                json.dump(report, f, indent=2, default=str)
            
            logger.info(f"Saved health report to {output_path}")
            
        except Exception as e:
            logger.error(f"Failed to save health report: {e}")
    
    def generate_metrics_dashboard(self) -> Dict[str, Any]:
        """Generate Grafana-compatible dashboard configuration."""
        dashboard = {
            "dashboard": {
                "title": "Bitunix Bot - System Health",
                "panels": [
                    # System metrics
                    {
                        "title": "CPU Usage",
                        "type": "gauge",
                        "targets": [{
                            "expr": "system_cpu_usage_percent"
                        }]
                    },
                    {
                        "title": "Memory Usage",
                        "type": "gauge",
                        "targets": [{
                            "expr": "system_memory_usage_percent"
                        }]
                    },
                    # Trading metrics
                    {
                        "title": "Active Positions",
                        "type": "stat",
                        "targets": [{
                            "expr": "active_positions"
                        }]
                    },
                    {
                        "title": "Total P&L",
                        "type": "stat",
                        "targets": [{
                            "expr": "pnl_total"
                        }]
                    },
                    # ML metrics
                    {
                        "title": "ML Model Accuracy",
                        "type": "gauge",
                        "targets": [{
                            "expr": "ml_accuracy"
                        }]
                    },
                    {
                        "title": "ML Prediction Latency",
                        "type": "graph",
                        "targets": [{
                            "expr": "rate(ml_latency_seconds_sum[5m]) / rate(ml_latency_seconds_count[5m])"
                        }]
                    },
                    # Error metrics
                    {
                        "title": "Error Rate",
                        "type": "graph",
                        "targets": [{
                            "expr": "rate(errors_total[5m])"
                        }]
                    }
                ]
            }
        }
        
        return dashboard

def main():
    """Main monitoring function."""
    parser = argparse.ArgumentParser(description="Enhanced Monitoring and Observability")
    parser.add_argument("--metrics-url", default="http://localhost:8080/metrics", 
                       help="Bot metrics endpoint URL")
    parser.add_argument("--prometheus-url", default="http://localhost:9090",
                       help="Prometheus server URL")
    parser.add_argument("--alert-config", help="Path to alert rules configuration")
    parser.add_argument("--output-dir", default="./reports", help="Output directory for reports")
    parser.add_argument("--interval", type=int, default=60, help="Monitoring interval in seconds")
    parser.add_argument("--daemon", action="store_true", help="Run in daemon mode")
    
    args = parser.parse_args()
    
    # Create output directory
    output_dir = Path(args.output_dir)
    output_dir.mkdir(exist_ok=True)
    
    # Initialize components
    metrics_collector = MetricsCollector(args.metrics_url)
    alert_manager = AlertManager(args.alert_config)
    dashboard = MonitoringDashboard(metrics_collector, alert_manager)
    
    logger.info("Enhanced monitoring started")
    
    try:
        if args.daemon:
            # Daemon mode - continuous monitoring
            logger.info(f"Running in daemon mode with {args.interval}s interval")
            
            while True:
                try:
                    # Generate health report
                    report = dashboard.generate_health_report()
                    
                    # Save report
                    timestamp = datetime.now().strftime('%Y%m%d_%H%M%S')
                    report_path = output_dir / f"health_report_{timestamp}.json"
                    dashboard.save_report(report, str(report_path))
                    
                    # Log status
                    status = report['status']
                    active_alerts = report['active_alerts']
                    metrics_count = report['metrics_count']
                    
                    logger.info(f"Health check: {status} | Metrics: {metrics_count} | Active alerts: {active_alerts}")
                    
                    # Wait for next interval
                    time.sleep(args.interval)
                    
                except KeyboardInterrupt:
                    logger.info("Monitoring interrupted by user")
                    break
                except Exception as e:
                    logger.error(f"Monitoring cycle failed: {e}")
                    time.sleep(args.interval)
        else:
            # Single run mode
            logger.info("Running single health check")
            
            report = dashboard.generate_health_report()
            
            # Save report
            timestamp = datetime.now().strftime('%Y%m%d_%H%M%S')
            report_path = output_dir / f"health_report_{timestamp}.json"
            dashboard.save_report(report, str(report_path))
            
            # Print summary
            print(f"\n=== Health Report Summary ===")
            print(f"Status: {report['status']}")
            print(f"Metrics collected: {report['metrics_count']}")
            print(f"Active alerts: {report['active_alerts']}")
            print(f"New alerts: {report['new_alerts']}")
            print(f"Report saved to: {report_path}")
            
            if report['active_alerts'] > 0:
                print(f"\n=== Active Alerts ===")
                for alert in report['alerts']:
                    print(f"- {alert['severity'].upper()}: {alert['description']}")
                    print(f"  {alert['metric_name']}: {alert['current_value']:.2f} (threshold: {alert['threshold']:.2f})")
                
                sys.exit(1)  # Exit with error code if alerts are active
    
    except Exception as e:
        logger.error(f"Monitoring failed: {e}")
        sys.exit(1)

if __name__ == "__main__":
    main()
