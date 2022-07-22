# 🤖 Liquidity Bot

This bot is a trading bot which aims to keep a flat position while still supplying liquidity to a market, all while attempting to steer the market to the same price as an external reference market.

### Pre-requisites

A running vega network<br>
An external market price source

When the bot is started it performs the following steps:

1. The bot will check that the configured user has enough assets to trade and enough tokens to raise a proposal. If not it will create some and deposit them into the users accounts.
1. The LB will look for a market with matching details to what is given in the configuration file. If it finds one it will use that for all trading. Otherwise it will create a new market to trade on. Creating a new market requires a proposal to be sent and then other users must vote to get the proposal passed. After a period of time defined in the proposal the votes are counted and if they reach or pass the threshold required, the market will be enacted and will be placed into an opening auction ready for trading.
1. A liquidity provision command is sent into the market to establish a liquidity shape and commitment amount.
1. If the market is in auction it will attempt to send LIMIT orders until the market is moved out of auction
1. Once the market is in continuous trading it will monitor the market and will perform two tasks<br>
 a. Price Steering<br>
 b. Position Management



## Price Steering
The test markets we are running aim to be close in price to the real markets of the World. The bot attempts to do this by monitoring the mark price of the Vega market and the price of an established market. If the prices are more than a specified amount apart it will start to place orders onto the Vega market to move the price towards the external price. If the external price is higher it will send buy orders to the market in an attempt to increase the best bid/ask price. If the external price is lower we will do the opposite by placing sell orders to attempt to reduce the best bid/ask price. The size of the order is controlled by a configuration option.


## Position Management
The bot wants to keep a flat (0) position to reduce the need for a large margin and to reduce risk. It tries to do this in 2 ways. First way is to use 2 different liquidity shapes depending on whether we are trying to reduce our position or increase it. When we have a negative position, we can alter our liquidity shape to put more buy orders near the best bid price and keep the sell orders away from the best ask price. If the market is moving randomly it should result in more of the liquidity buy orders being filled which will increase our position. We do the opposite when our position is positive to increase the chance our sell orders are matched instead of the buy orders.

The second process the bots use is to place market orders specifically to reduce/increase the position when a threshold is reached. If we set a maximum long position and we reach that amount, the bot will place a sell order to reduce its position. The same process is performed if the position gets too short, it will place a buy order to reduce its shortness. The trigger limit for maximum short and long position is configurable along with the size of the order placed when the limit is reached.

## Licence

Distributed under the MIT License. See `LICENSE` for more information.
