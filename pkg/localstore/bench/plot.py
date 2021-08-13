import matplotlib as mpl
import pandas as pd
from matplotlib import pyplot as plt
g1 = pd.read_csv("in-mem/get_req_reserve_size.csv", usecols=['res','getrtime'])
g2 = pd.read_csv("on-disk/get_req_reserve_size.csv", usecols=['res','getrtime'])
g3 = pd.read_csv("8-byte/get_req_reserve_size.csv", usecols=['res','getrtime'])
p1 = pd.read_csv("in-mem/put_up_reserve_size.csv", usecols=['res','pututime'])
p2 = pd.read_csv("on-disk/put_up_reserve_size.csv", usecols=['res','pututime'])
p3 = pd.read_csv("8-byte/put_up_reserve_size.csv", usecols=['res','pututime'])
t1 = pd.read_csv("in-mem/put_up_time.csv", usecols=['chunks','put100time'])
t2 = pd.read_csv("on-disk/put_up_time.csv", usecols=['chunks','put100time'])
t3 = pd.read_csv("8-byte/put_up_time.csv", usecols=['chunks','put100time'])

period=30

cfg=3
if cfg == 0:
    plt.plot(g1.res,g1.getrtime/1000,label="get request time(in-mem)")
    plt.plot(g2.res,g2.getrtime/1000,label="get request time(on-disk)")
    plt.plot(g3.res,g3.getrtime/1000,label="get request time(8byte)")
    plt.ylabel('time(millisecond)')
    plt.title("random get time")

if cfg == 1:
    plt.plot(p1.res,p1.pututime/1000,label="put request time(in-mem)")
    plt.plot(p2.res,p2.pututime/1000,label="put request time(on-disk)")
    plt.plot(p3.res,p3.pututime/1000,label="put request time(8byte)")
    plt.ylabel('time(millisecond)')
    plt.title("put time")

if cfg == 2:
    plt.plot(p1.res,p1.pututime/1000,label="put request time(in-mem)")
    plt.plot(p2.res,p2.pututime/1000,label="put request time(on-disk)")
    plt.plot(p3.res,p3.pututime/1000,label="put request time(8byte)")
    m1 = p1.pututime.rolling(window=period).median()
    m2 = p2.pututime.rolling(window=period).median()
    m3 = p3.pututime.rolling(window=period).median()
    plt.plot(p1.res,m1/1000,label="put request time(in-mem), mean 30")
    plt.plot(p2.res,m2/1000,label="put request time(on-disk), mean 30")
    plt.plot(p3.res,m3/1000,label="put request time(8byte), mean 30")
    plt.ylabel('time(millisecond)')
    plt.title("put time")

if cfg == 3:
    plt.plot(t1.chunks,t1.put100time/1000,label="put 1k chunks time(in-mem)")
    plt.plot(t2.chunks,t2.put100time/1000,label="put 1k chunks time(on-disk)")
    plt.plot(t3.chunks,t3.put100time/1000,label="put 1k chunks time(8byte)")
    plt.ylabel('time(second)')
    plt.title("put 4 meg time")

plt.xlabel('size')
plt.legend()
plt.show(block=True)

