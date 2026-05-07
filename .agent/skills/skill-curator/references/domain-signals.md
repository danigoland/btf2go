# Domain Signals

Use these hints when inferring domain from the repository or spec files.

## AI agent development

Strong signals:
- langchain, langgraph, autogen, crewai, semantic kernel
- openai sdk usage tied to tools, planners, memory, retrieval, evals, traces
- terms like agent, tool calling, planner, evaluator, memory, rag, workflow, orchestration

Recommended adds after baseline:
- evals
- observability or tracing
- agent architecture
- retrieval or rag when central

## Finance / quant

Strong signals:
- backtest, alpha, factor, portfolio, pnl, market data, execution, slippage, risk
- pandas/numpy heavy pipelines tied to trading or analytics
- spec language about strategies, signal generation, backtesting, validation

Recommended adds after baseline:
- quant analysis
- backtesting
- data validation
- profiling or performance when compute heavy

## Scientific / research code

Strong signals:
- notebooks, experiments, scipy, simulation, reproducibility, benchmark, model fitting
- experiment logs, parameter sweeps, results directories

Recommended adds after baseline:
- reproducibility
- numerical analysis
- profiling
- data validation
