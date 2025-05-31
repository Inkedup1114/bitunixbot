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
    """Collects and processes metrics from various sources."""
    
    def __init__(self, metrics_url: str = "http://localhost:8080/metrics"):
        self.metrics_url = metrics_url
        self.session = requests.Session()
        self.session.timeout = 10
    
    def collect_bot_metrics(self) -> List[MetricValue]:
        """Collect metrics from bot metrics endpoint."""
        try:
            response = self.session.get(self.metrics_url)
            response.raise_for_status()
            
            metrics = []
            for family in text_string_to_metric_families(response.text):
                for sample in family.samples:
                    metric = MetricValue(
                        name=sample.name,
                        value=sample.value,
                        labels=dict(sample.labels),
                        timestamp=datetime.now(),
                        help_text=family.documentation
                    )
                    metrics.append(metric)
            
            logger.info(f"Collected {len(metrics)} metrics from bot")
            return metrics
            
        except Exception as e:
            logger.error(f"Failed to collect bot metrics: {e}")
            return []
    
    def collect_system_metrics(self) -> List[MetricValue]:
        """Collect system-level metrics."""
        metrics = []
        timestamp = datetime.now()
        
        try:
            # CPU usage
            cpu_usage = self._get_cpu_usage()
            if cpu_usage is not None:
                metrics.append(MetricValue(
                    name="system_cpu_usage_percent",
                    value=cpu_usage,
                    labels={},
                    timestamp=timestamp,
                    help_text="System CPU usage percentage"
                ))
            
            # Memory usage
            memory_usage = self._get_memory_usage()
            if memory_usage is not None:
                metrics.append(MetricValue(
                    name="system_memory_usage_percent",
                    value=memory_usage,
                    labels={},
                    timestamp=timestamp,
                    help_text="System memory usage percentage"
                ))
            
            # Disk usage
            disk_usage = self._get_disk_usage()
            if disk_usage is not None:
                metrics.append(MetricValue(
                    name="system_disk_usage_percent",
                    value=disk_usage,
                    labels={},
                    timestamp=timestamp,
                    help_text="System disk usage percentage"
                ))
            
            logger.info(f"Collected {len(metrics)} system metrics")
            return metrics
            
        except Exception as e:
            logger.error(f"Failed to collect system metrics: {e}")
            return []
    
    def _get_cpu_usage(self) -> Optional[float]:
        """Get CPU usage percentage."""
        try:
            result = subprocess.run(
                ["top", "-bn1"], 
                capture_output=True, 
                text=True, 
                timeout=5
            )
            
            for line in result.stdout.split('\n'):
                if '%Cpu(s):' in line:
                    # Extract idle percentage and calculate usage
                    parts = line.split(',')
                    for part in parts:
                        if 'id' in part:
                            idle = float(part.strip().split()[0])
                            return 100 - idle
            return None
            
        except Exception as e:
            logger.warning(f"CPU usage collection failed: {e}")
            return None
    
    def _get_memory_usage(self) -> Optional[float]:
        """Get memory usage percentage."""
        try:
            result = subprocess.run(
                ["free", "-m"], 
                capture_output=True, 
                text=True, 
                timeout=5
            )
            
            lines = result.stdout.split('\n')
            for line in lines:
                if line.startswith('Mem:'):
                    parts = line.split()
                    total = int(parts[1])
                    used = int(parts[2])
                    return (used / total) * 100
            return None
            
        except Exception as e:
            logger.warning(f"Memory usage collection failed: {e}")
            return None
    
    def _get_disk_usage(self) -> Optional[float]:
        """Get disk usage percentage."""
        try:
            result = subprocess.run(
                ["df", "-h", "/"], 
                capture_output=True, 
                text=True, 
                timeout=5
            )
            
            lines = result.stdout.split('\n')
            if len(lines) >= 2:
                parts = lines[1].split()
                usage_str = parts[4].rstrip('%')
                return float(usage_str)
            return None
            
        except Exception as e:
            logger.warning(f"Disk usage collection failed: {e}")
            return None

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
        """Send Slack notification."""
        try:
            emoji = "🔴" if alert.severity == "critical" else "⚠️"
            color = "danger" if alert.severity == "critical" else "warning"
            
            payload = {
                "text": f"{emoji} Bitunix Bot Alert",
                "attachments": [{
                    "color": color,
                    "fields": [
                        {
                            "title": "Alert",
                            "value": alert.description,
                            "short": False
                        },
                        {
                            "title": "Metric",
                            "value": f"{alert.metric_name}: {alert.current_value:.2f}",
                            "short": True
                        },
                        {
                            "title": "Threshold",
                            "value": f"{alert.threshold:.2f}",
                            "short": True
                        },
                        {
                            "title": "Severity",
                            "value": alert.severity.upper(),
                            "short": True
                        },
                        {
                            "title": "Time",
                            "value": alert.timestamp.strftime("%Y-%m-%d %H:%M:%S"),
                            "short": True
                        }
                    ]
                }]
            }
            
            response = requests.post(webhook_url, json=payload, timeout=10)
            response.raise_for_status()
            logger.info("Slack notification sent successfully")
            
        except Exception as e:
            logger.error(f"Failed to send Slack notification: {e}")
    
    def _send_email_notification(self, alert: Alert, email_config: Dict):
        """Send email notification."""
        try:
            import smtplib
            from email.mime.text import MimeText
            from email.mime.multipart import MimeMultipart
            
            msg = MimeMultipart()
            msg['From'] = email_config['from']
            msg['To'] = email_config['to']
            msg['Subject'] = f"Bitunix Bot Alert: {alert.description}"
            
            body = f"""
Alert Details:
- Rule: {alert.rule_name}
- Description: {alert.description}
- Metric: {alert.metric_name}
- Current Value: {alert.current_value:.2f}
- Threshold: {alert.threshold:.2f}
- Severity: {alert.severity.upper()}
- Timestamp: {alert.timestamp.strftime("%Y-%m-%d %H:%M:%S")}

Please investigate this issue promptly.
            """
            
            msg.attach(MimeText(body, 'plain'))
            
            server = smtplib.SMTP(email_config['smtp_host'], email_config['smtp_port'])
            if email_config.get('use_tls', False):
                server.starttls()
            if email_config.get('username') and email_config.get('password'):
                server.login(email_config['username'], email_config['password'])
            
            server.send_message(msg)
            server.quit()
            
            logger.info("Email notification sent successfully")
            
        except Exception as e:
            logger.error(f"Failed to send email notification: {e}")
    
    def _get_email_config(self) -> Optional[Dict]:
        """Get email configuration from environment variables."""
        config = {}
        
        config['from'] = os.getenv('ALERT_EMAIL_FROM')
        config['to'] = os.getenv('ALERT_EMAIL_TO')
        config['smtp_host'] = os.getenv('ALERT_SMTP_HOST')
        config['smtp_port'] = int(os.getenv('ALERT_SMTP_PORT', '587'))
        config['username'] = os.getenv('ALERT_SMTP_USERNAME')
        config['password'] = os.getenv('ALERT_SMTP_PASSWORD')
        config['use_tls'] = os.getenv('ALERT_SMTP_TLS', 'true').lower() == 'true'
        
        # Check if required fields are present
        required_fields = ['from', 'to', 'smtp_host']
        if all(config.get(field) for field in required_fields):
            return config
        
        return None

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
        """Generate summary statistics."""
        summary = {}
        
        # Key metrics summary
        key_metrics = [
            'trading_orders_total',
            'trading_profits_total',
            'ml_predictions_total',
            'ml_accuracy',
            'websocket_reconnections_total',
            'system_cpu_usage_percent',
            'system_memory_usage_percent'
        ]
        
        for metric_name in key_metrics:
            metric = next((m for m in metrics if m.name == metric_name), None)
            if metric:
                summary[metric_name] = metric.value
        
        return summary
    
    def save_report(self, report: Dict[str, Any], output_path: str = None):
        """Save health report to file."""
        try:
            if output_path is None:
                timestamp = datetime.now().strftime('%Y%m%d_%H%M%S')
                output_path = f"health_report_{timestamp}.json"
            
            with open(output_path, 'w') as f:
                json.dump(report, f, indent=2, default=str)
            
            logger.info(f"Health report saved to {output_path}")
            
        except Exception as e:
            logger.error(f"Failed to save health report: {e}")

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
