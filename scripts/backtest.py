import pandas as pd
import numpy as np
import onnxruntime as ort

def load_model(model_path):
    session = ort.InferenceSession(model_path)
    return session

def run_backtest(df, model_session):
    initial_balance = 10000
    balance = initial_balance
    position = 0
    entry_price = 0
    trades = []

    for idx, row in df.iterrows():
        features = np.array([[row['tick_ratio'], row['depth_ratio'], row['price_dist']]], dtype=np.float32)
        prediction = model_session.run(None, {model_session.get_inputs()[0].name: features})[1][0][1]

        # Simple threshold-based entry/exit logic
        if prediction > 0.6 and position == 0:
            position = balance / row['price']
            entry_price = row['price']
            trades.append(('BUY', row['timestamp'], entry_price))
        elif prediction < 0.4 and position > 0:
            balance = position * row['price']
            trades.append(('SELL', row['timestamp'], row['price']))
            position = 0

    # Close any open positions at the end
    if position > 0:
        balance = position * df.iloc[-1]['price']
        trades.append(('SELL', df.iloc[-1]['timestamp'], df.iloc[-1]['price']))

    pnl = balance - initial_balance
    print(f"Final Balance: ${balance:.2f}, PnL: ${pnl:.2f}")
    print("Trades executed:")
    for trade in trades:
        print(trade)

if __name__ == "__main__":
    df = pd.read_csv("historical_data.csv")
    model_session = load_model("models/model.onnx")
    run_backtest(df, model_session)
