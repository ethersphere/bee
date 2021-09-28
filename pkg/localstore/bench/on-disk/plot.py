import matplotlib as mpl
import pandas as pd
from matplotlib import pyplot as plt
columns=['res','getrtime']
columns2=['res','pututime']
df = pd.read_csv("get_req_reserve_size.csv", usecols=columns)
df2 = pd.read_csv("put_up_reserve_size.csv", usecols=columns2)
df3 = pd.read_csv("put_up_time.csv", usecols=['chunks','put100time'])
# mpl.rcParams['path.simplify'] = True
# mpl.rcParams['path.simplify_threshold'] = 0.5

period=30
rolling_meang = df.getrtime.rolling(window=period).median()
rolling_meanp = df2.pututime.rolling(window=period).median()

# plt.plot(df.res, df.getrtime,label="get raw")
# plt.plot(df2.res, df2.pututime,label="put raw")
# plt.plot(df.res, rolling_meang,label="get mean 30")
# plt.plot(df2.res, rolling_meanp,label="put mean 30")
plt.plot(df3.chunks,df3.put100time,label="put1k chunks")

plt.xlabel('size')
# plt.ylabel('time(microseconds)')
plt.ylabel('time(millisec)')
plt.title("get/put time")
plt.legend()
plt.show(block=True)

